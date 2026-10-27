package queue

import (
	"context"
	"log/slog"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
)

func TestSecuritySelectionRecheckedBeforeDispatch(t *testing.T) {
	d := pgtest.Open(t)
	ctx := context.Background()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	hostID, packageID, runID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO hosts (id,friendly_name,os_type,os_version,api_id,api_key,package_manager,updated_at) VALUES ($1,'security-test','Ubuntu','24.04',$1,$1,'apt',NOW())`, hostID)
	exec(`INSERT INTO packages (id,name,latest_version,updated_at) VALUES ($1,'openssl','2',NOW())`, packageID)
	exec(`INSERT INTO host_packages (id,host_id,package_id,current_version,available_version,needs_update,is_security_update,last_checked) VALUES ($1,$2,$3,'1','2',true,true,NOW())`, uuid.NewString(), hostID, packageID)
	runs := store.NewPatchRunsStore(d)
	_, err := runs.CreateRun(ctx, runID, hostID, "patch-run-"+runID, "patch_package", nil, []string{"openssl"}, nil, false, nil, nil, nil, store.WithPatchSelectionMode([]byte(`{}`), "security"), nil)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE host_packages SET is_security_update=false WHERE host_id=$1`, hostID)
	registry := agentregistry.New()
	registry.Register(hostID, true, "")
	h := NewRunPatchHandler(registry, runs, nil, nil, slog.Default())
	task, err := NewRunPatchTask(RunPatchPayload{HostID: hostID, ApiID: hostID, PatchRunID: runID, PatchType: "patch_package", PackageNames: []string{"openssl"}})
	if err != nil {
		t.Fatal(err)
	}
	// There is no WebSocket or command runner. If validation lets this task
	// reach SendMessage the call fails, making an accidental dispatch visible.
	if err := h.ProcessTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByID(ctx, runID)
	if err != nil || run == nil || run.Status != "failed" {
		t.Fatalf("%+v %v", run, err)
	}
}
