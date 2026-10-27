package handler

import (
	"context"
	"net/http"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

type patchSelectionHost struct {
	ID           string   `json:"id"`
	FriendlyName string   `json:"friendly_name"`
	Hostname     *string  `json:"hostname"`
	OSType       string   `json:"os_type"`
	Ready        bool     `json:"ready"`
	Reason       string   `json:"reason,omitempty"`
	PackageNames []string `json:"package_names,omitempty"`
}

// SelectionPreview resolves explicit IDs, including selections off the current
// page. It does not create validation runs, patch runs or deferred work.
func (h *PatchingHandler) SelectionPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		HostIDs []string `json:"host_ids"`
		Mode    string   `json:"mode"`
	}
	if err := decodeJSON(r, &body); err != nil || (body.Mode != "all" && body.Mode != "security") || len(body.HostIDs) == 0 || len(body.HostIDs) > 200 {
		Error(w, http.StatusBadRequest, "Select 1 to 200 hosts and a valid update scope")
		return
	}
	for _, id := range body.HostIDs {
		if !isValidPatchUUID(id) {
			Error(w, http.StatusBadRequest, "Invalid host ID")
			return
		}
	}
	out := []patchSelectionHost{}
	seen := map[string]bool{}
	for _, id := range body.HostIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		row := patchSelectionHost{ID: id, FriendlyName: id}
		host, err := h.hosts.GetByID(r.Context(), id)
		if err != nil || host == nil {
			row.Reason = "Host not found or unavailable in this context"
			out = append(out, row)
			continue
		}
		row.FriendlyName, row.Hostname, row.OSType = host.FriendlyName, host.Hostname, host.OSType
		row.Reason = h.selectionReadiness(r.Context(), host)
		if row.Reason == "" {
			active, err := h.hosts.HasActivePatchRun(r.Context(), id)
			if err != nil {
				row.Reason = "Unable to check patch activity"
			} else if active {
				row.Reason = activePatchRunMessage
			}
		}
		if row.Reason == "" && body.Mode == "security" {
			row.PackageNames, err = h.patchRuns.SecurityPackageNames(r.Context(), host)
			if err != nil {
				row.Reason = err.Error()
			}
		}
		if row.Reason == "" && body.Mode == "all" && store.IsWindowsOSType(host.OSType) && windowsAgentVersionError(host.AgentVersion) != nil {
			row.Reason = "Update this agent before patching Windows"
		}
		if row.Reason == "" && body.Mode == "all" {
			updates, err := h.patchRuns.HasReportedUpdates(r.Context(), id)
			if err != nil {
				row.Reason = "Unable to read update inventory"
			} else if !updates {
				row.Reason = "No reported updates; fetch a fresh report to check"
			}
		}
		row.Ready = row.Reason == ""
		out = append(out, row)
	}
	JSON(w, http.StatusOK, map[string]interface{}{"hosts": out})
}

func (h *PatchingHandler) selectionReadiness(ctx context.Context, host *models.Host) string {
	if host.IdentityConflictAt != nil || host.HostIDMismatch() {
		return "Resolve the host identity conflict before patching"
	}
	if host.AwaitingPostPatchReportRunID != nil {
		return "Waiting for a fresh post-patch report before patching again"
	}
	if h.registry == nil || !h.registry.IsConnected(host.ApiID) || h.registry.Get(host.ApiID).Scope != hostctx.TenantHostKey(ctx) {
		return "Agent offline — waiting for the agent to reconnect."
	}
	if h.selectionReboots != nil {
		state, err := h.selectionReboots.stateForHost(ctx, host)
		if err != nil {
			return "Unable to check reboot activity"
		}
		if state != nil && state.Status != "completed" && state.Status != "failed" {
			return "A reboot request is outstanding; check the host before patching"
		}
	}
	return ""
}
