package store

import (
	"slices"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestSecurityPackageSelection(t *testing.T) {
	now := time.Now()
	manager := "apt"
	host := &models.Host{OSType: "Ubuntu", PackageManager: &manager}
	row := db.GetHostPackagesWithPackagesRow{PkgName: "openssl", NeedsUpdate: true, IsSecurityUpdate: true, LastChecked: pgtype.Timestamp{Time: now, Valid: true}}
	regular := row
	regular.PkgName, regular.IsSecurityUpdate = "curl", false
	current := row
	current.PkgName, current.NeedsUpdate = "sudo", false
	names, err := securityPackageNames(host, []db.GetHostPackagesWithPackagesRow{regular, row, row, current}, now)
	if err != nil || !slices.Equal(names, []string{"openssl"}) {
		t.Fatalf("%v %v", names, err)
	}
	for _, manager := range []string{"apt", "dnf", "yum", "apk", "zypper"} {
		host.PackageManager = &manager
		if _, err := securityPackageNames(host, []db.GetHostPackagesWithPackagesRow{row}, now); err != nil {
			t.Fatal(manager, err)
		}
	}
	for _, manager := range []string{"pacman", "unknown", ""} {
		host.PackageManager = &manager
		if _, err := securityPackageNames(host, []db.GetHostPackagesWithPackagesRow{row}, now); err == nil {
			t.Fatal("accepted unsupported manager", manager)
		}
	}
	host.PackageManager = &manager
	for _, os := range []string{"Windows Server", "FreeBSD"} {
		host.OSType = os
		if _, err := securityPackageNames(host, []db.GetHostPackagesWithPackagesRow{row}, now); err == nil {
			t.Fatal("accepted unsupported OS", os)
		}
	}
	host.OSType = "Ubuntu"
	for _, stamp := range []pgtype.Timestamp{{}, {Valid: true, Time: now.Add(-25 * time.Hour)}, {Valid: true, Time: now.Add(time.Hour)}} {
		stale := row
		stale.LastChecked = stamp
		if _, err := securityPackageNames(host, []db.GetHostPackagesWithPackagesRow{stale}, now); err == nil {
			t.Fatal("accepted stale/future inventory")
		}
	}
	if _, err := securityPackageNames(host, []db.GetHostPackagesWithPackagesRow{regular, current}, now); err == nil {
		t.Fatal("accepted missing security metadata")
	}
}

func TestFixedSecurityNamesAndSnapshot(t *testing.T) {
	if err := checkSecurityNames([]string{"openssl"}, []string{"curl", "openssl"}); err != nil {
		t.Fatal(err)
	}
	for _, names := range [][]string{nil, {"curl"}, {"openssl", "regular"}} {
		if err := checkSecurityNames(names, []string{"openssl"}); err == nil {
			t.Fatal("accepted widened/empty scope", names)
		}
	}
	raw := WithPatchSelectionMode([]byte(`{"name":"nightly","delay_minutes":30}`), "security")
	if PatchSelectionMode(raw) != "security" {
		t.Fatal(string(raw))
	}
	if PatchSelectionMode([]byte(`{"name":"old policy"}`)) != "" {
		t.Fatal("changed existing runs")
	}
}
