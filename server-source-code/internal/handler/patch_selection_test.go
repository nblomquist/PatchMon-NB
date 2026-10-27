package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestPatchSelectionPreviewAndSecurityTrigger(t *testing.T) {
	d := pgtest.Open(t)
	ctx := context.Background()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	hosts := store.NewHostsStore(d)
	runs := store.NewPatchRunsStore(d)
	registry := agentregistry.New()
	h := NewPatchingHandler(runs, store.NewPatchPoliciesStore(d), nil, nil, hosts, nil, nil, nil, nil, nil, nil)
	h.SetStreamDependencies(nil, registry)
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for i, id := range ids {
		exec(`INSERT INTO hosts (id, friendly_name, os_type, os_version, api_id, api_key, package_manager, agent_version, updated_at) VALUES ($1,$2,'Ubuntu','24.04',$1,$1,'apt','2.1.6~nb1',NOW())`, id, "test-host-"+id)
		if i < 2 {
			registry.Register(id, true, "")
		}
	}
	for i, name := range []string{"openssl", "sudo", "regular"} {
		pkg := uuid.NewString()
		exec(`INSERT INTO packages (id,name,latest_version,updated_at) VALUES ($1,$2,'2',NOW())`, pkg, name)
		exec(`INSERT INTO host_packages (id,host_id,package_id,current_version,available_version,needs_update,is_security_update,last_checked) VALUES ($1,$2,$3,'1','2',true,$4,NOW())`, uuid.NewString(), ids[i%2], pkg, i < 2)
	}
	request := func(path string, body interface{}, contextOverride context.Context) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw))).WithContext(contextOverride)
		w := httptest.NewRecorder()
		if path == "/preview" {
			h.SelectionPreview(w, r)
		} else {
			h.Trigger(w, r)
		}
		return w
	}
	w := request("/preview", map[string]interface{}{"host_ids": ids, "mode": "security"}, ctx)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var preview struct {
		Hosts []patchSelectionHost `json:"hosts"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &preview)
	if len(preview.Hosts) != 3 || !preview.Hosts[0].Ready || !preview.Hosts[1].Ready || preview.Hosts[2].Ready {
		t.Fatal(w.Body.String())
	}
	if strings.Join(preview.Hosts[0].PackageNames, ",") != "openssl" || strings.Join(preview.Hosts[1].PackageNames, ",") != "sudo" {
		t.Fatal("host package union leaked", w.Body.String())
	}
	registry.Register(ids[2], true, "")
	w = request("/preview", map[string]interface{}{"host_ids": append(ids, ids[0]), "mode": "all"}, ctx)
	_ = json.Unmarshal(w.Body.Bytes(), &preview)
	if w.Code != 200 || len(preview.Hosts) != 3 || !preview.Hosts[0].Ready || preview.Hosts[2].Ready || !strings.Contains(preview.Hosts[2].Reason, "No reported updates") {
		t.Fatal(w.Body.String())
	}
	other := hostctx.WithEntry(ctx, &hostctx.Entry{Host: "other.example"})
	w = request("/preview", map[string]interface{}{"host_ids": ids[:1], "mode": "all"}, other)
	if strings.Contains(w.Body.String(), `"ready":true`) {
		t.Fatal("cross-scope connection accepted")
	}
	for _, tc := range []struct {
		names     []string
		patchType string
		want      int
	}{{[]string{"regular"}, "patch_package", 409}, {[]string{"sudo"}, "patch_package", 409}, {nil, "patch_package", 409}, {nil, "patch_all", 400}, {[]string{"openssl"}, "patch_package", 200}} {
		w = request("/trigger", map[string]interface{}{"host_id": ids[0], "patch_type": tc.patchType, "package_names": tc.names, "selection_mode": "security", "pending_approval": true}, ctx)
		if w.Code != tc.want {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
	var created struct {
		ID string `json:"patch_run_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	run, err := runs.GetByID(ctx, created.ID)
	if err != nil || run == nil || run.Status != "pending_approval" || store.PatchSelectionMode(run.PolicySnapshot) != "security" || string(run.PackageNames) != `["openssl"]` {
		t.Fatalf("%+v %v", run, err)
	}
	// Reclassification between review and approval must prevent a release.
	exec(`UPDATE host_packages SET is_security_update=false WHERE host_id=$1`, ids[0])
	router := chi.NewRouter()
	router.Post("/runs/{id}/approve", h.ApproveRun)
	r := httptest.NewRequest("POST", "/runs/"+created.ID+"/approve", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("approval widened scope", w.Code, w.Body.String())
	}
	exec(`UPDATE patch_runs SET status='queued' WHERE id=$1`, created.ID)
	w = request("/preview", map[string]interface{}{"host_ids": ids[:1], "mode": "all"}, ctx)
	_ = json.Unmarshal(w.Body.Bytes(), &preview)
	if w.Code != 200 || preview.Hosts[0].Ready || preview.Hosts[0].Reason != activePatchRunMessage {
		t.Fatal("active run not excluded", w.Body.String())
	}
}

func TestPatchSelectionReadiness(t *testing.T) {
	reboots, hosts, _, _ := rebootFixture(t)
	h := &PatchingHandler{registry: reboots.registry, selectionReboots: reboots}
	ctx := context.Background()
	if reason := h.selectionReadiness(ctx, hosts.host); reason != "" {
		t.Fatal(reason)
	}
	if reason := h.selectionReadiness(hostctx.WithEntry(ctx, &hostctx.Entry{Host: "another.example"}), hosts.host); reason == "" {
		t.Fatal("wrong-scope agent accepted")
	}
	marker := "previous-run"
	hosts.host.AwaitingPostPatchReportRunID = &marker
	if reason := h.selectionReadiness(ctx, hosts.host); reason == "" {
		t.Fatal("stale post-patch report accepted")
	}
	hosts.host.AwaitingPostPatchReportRunID = nil
	w := rebootHTTP(reboots, "POST", hosts.host.ID, `{"confirmed":true}`, "operator", ctx)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if reason := h.selectionReadiness(ctx, hosts.host); reason == "" {
		t.Fatal("outstanding reboot accepted")
	}
}

func TestPatchSelectionPermission(t *testing.T) {
	d := pgtest.Open(t)
	permissions := store.NewPermissionsStore(d)
	for _, role := range []string{"", "user"} {
		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(204) })
		r := httptest.NewRequest("POST", "/patching/selection-preview", nil)
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserRoleKey, role))
		w := httptest.NewRecorder()
		middleware.RequirePermission("can_manage_patching", permissions)(next).ServeHTTP(w, r)
		if called || w.Code < 400 {
			t.Fatalf("role=%s status=%d", role, w.Code)
		}
	}
}

func TestPatchSelectionInvalidInput(t *testing.T) {
	h := &PatchingHandler{}
	for _, body := range []string{`{}`, `{"mode":"all","host_ids":["bad"]}`, `{"mode":"invalid","host_ids":["bad"]}`} {
		w := httptest.NewRecorder()
		h.SelectionPreview(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	raw, _ := json.Marshal(map[string]interface{}{"mode": "all", "host_ids": ids})
	w := httptest.NewRecorder()
	h.SelectionPreview(w, httptest.NewRequest("POST", "/", strings.NewReader(string(raw))))
	if w.Code != 400 {
		t.Fatal("oversize selection accepted")
	}
}
