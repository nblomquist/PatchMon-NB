package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// Security inventory is a report, not a live package-manager query. Never use
// heartbeat freshness as a substitute for the packages' own last_checked time.
const SecurityInventoryMaxAge = 24 * time.Hour

func securityPackageNames(host *models.Host, rows []db.GetHostPackagesWithPackagesRow, now time.Time) ([]string, error) {
	if host.IdentityConflictAt != nil || host.HostIDMismatch() || host.AwaitingPostPatchReportRunID != nil {
		return nil, errors.New("host identity or post-patch inventory requires review before security patching")
	}
	manager := ""
	if host.PackageManager != nil {
		manager = strings.ToLower(*host.PackageManager)
	}
	if RefusesSelectedPatch(host.PackageManager, "patch_package") {
		return nil, errors.New("security-only patching is unavailable on Arch Linux because partial upgrades are unsupported; use All updates")
	}
	// FreeBSD base updates are generically marked security-related by older
	// agents, rather than classified by advisory. Do not make that claim here.
	if IsWindowsOSType(host.OSType) || strings.Contains(strings.ToLower(host.OSType), "freebsd") {
		return nil, errors.New("security-only patching is not supported for this operating system yet; use the existing per-host update selection")
	}
	switch manager {
	case "apt", "dnf", "yum", "zypper", "apk":
	default:
		return nil, errors.New("security update classification is not supported for this package manager")
	}
	seen := map[string]bool{}
	names := []string{}
	for _, row := range rows {
		if !row.NeedsUpdate || !row.IsSecurityUpdate {
			continue
		}
		if !row.LastChecked.Valid || now.Sub(row.LastChecked.Time) > SecurityInventoryMaxAge || row.LastChecked.Time.After(now.Add(5*time.Minute)) {
			return nil, errors.New("security inventory is stale; fetch a fresh report before patching")
		}
		if !seen[row.PkgName] {
			seen[row.PkgName] = true
			names = append(names, row.PkgName)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return nil, errors.New("no reported security updates; classification may be unavailable; fetch a fresh report to check")
	}
	if len(names) > 100 {
		return nil, errors.New("more than 100 security packages; use smaller per-host package selections")
	}
	return names, nil
}

func (s *PatchRunsStore) SecurityPackageNames(ctx context.Context, host *models.Host) ([]string, error) {
	rows, err := s.db.DB(ctx).Queries.GetHostPackagesWithPackages(ctx, host.ID)
	if err != nil {
		return nil, fmt.Errorf("cannot read security inventory: %w", err)
	}
	return securityPackageNames(host, rows, time.Now())
}

func (s *PatchRunsStore) HasReportedUpdates(ctx context.Context, hostID string) (bool, error) {
	rows, err := s.db.DB(ctx).Queries.GetHostPackagesWithPackages(ctx, hostID)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row.NeedsUpdate {
			return true, nil
		}
	}
	return false, nil
}

// CheckSecuritySelection rechecks the fixed, confirmed package list. Newly
// reported security updates never expand an existing request or scheduled run.
func (s *PatchRunsStore) CheckSecuritySelection(ctx context.Context, hostID string, names []string) error {
	host, err := NewHostsStore(s.db).GetByID(ctx, hostID)
	if err != nil || host == nil {
		return errors.New("cannot resolve host for security patching")
	}
	eligible, err := s.SecurityPackageNames(ctx, host)
	if err != nil {
		return err
	}
	return checkSecurityNames(names, eligible)
}

func checkSecurityNames(names, eligible []string) error {
	if len(names) == 0 || len(names) > 100 {
		return errors.New("security-only patching requires 1 to 100 explicit package names")
	}
	for _, name := range names {
		if !slices.Contains(eligible, name) {
			return fmt.Errorf("security inventory changed for %s; review a fresh selection before patching", name)
		}
	}
	return nil
}

// Keep this additive marker in the existing policy snapshot so the constraint
// survives validation, approval and delayed dispatch without a schema change.
func PatchSelectionMode(snapshot []byte) string {
	var data struct {
		Mode string `json:"nb_selection_mode"`
	}
	_ = json.Unmarshal(snapshot, &data)
	return data.Mode
}

func WithPatchSelectionMode(snapshot []byte, mode string) []byte {
	if mode == "" {
		return snapshot
	}
	data := map[string]interface{}{}
	_ = json.Unmarshal(snapshot, &data)
	data["nb_selection_mode"] = mode
	raw, _ := json.Marshal(data)
	return raw
}
