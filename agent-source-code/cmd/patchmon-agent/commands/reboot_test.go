package commands

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func resetRebootGuard(t *testing.T) {
	t.Helper()
	patchUpdateMu.Lock()
	oldPending, oldRuns, oldUpdating := rebootPending, activePatchRuns, agentUpdating
	oldID := rebootPendingID
	rebootPending, activePatchRuns, agentUpdating = false, 0, false
	rebootPendingID = ""
	patchUpdateMu.Unlock()
	t.Cleanup(func() {
		patchUpdateMu.Lock()
		rebootPending, activePatchRuns, agentUpdating = oldPending, oldRuns, oldUpdating
		rebootPendingID = oldID
		patchUpdateMu.Unlock()
	})
}

func TestRebootPlatformCommands(t *testing.T) {
	for _, tc := range []struct {
		os, name string
		args     []string
	}{
		{"linux", "shutdown", []string{"-r", "now"}}, {"freebsd", "shutdown", []string{"-r", "now"}}, {"windows", "shutdown.exe", []string{"/r", "/t", "0"}},
	} {
		name, args, err := rebootCommand(tc.os)
		if err != nil || name != tc.name || !reflect.DeepEqual(args, tc.args) {
			t.Fatalf("%s: %s %v %v", tc.os, name, args, err)
		}
	}
	if _, _, err := rebootCommand("darwin"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestRebootAcceptedBeforeExecutionAndBlocksDuplicates(t *testing.T) {
	resetRebootGuard(t)
	var statuses []string
	calls := 0
	send := func(r rebootReply) error { statuses = append(statuses, r.Status); return nil }
	run := func() error {
		calls++
		if len(statuses) != 1 || statuses[0] != "accepted" {
			t.Fatal("execution before acknowledgement")
		}
		return nil
	}
	processReboot("test-request", time.Now().Add(30*time.Second), send, run, func() {})
	processReboot("test-request", time.Now().Add(30*time.Second), send, run, func() {})
	if calls != 1 || !reflect.DeepEqual(statuses, []string{"accepted", "requested", "accepted"}) {
		t.Fatalf("%d %v", calls, statuses)
	}
	if end, ok := beginPatchRun(); ok {
		end()
		t.Fatal("patching allowed during reboot")
	}
	if _, err := updateAgentUnlessPatching(); err == nil {
		t.Fatal("agent update allowed during reboot")
	}
}

func TestRebootGuardsAndFailures(t *testing.T) {
	for _, kind := range []string{"patching", "updating", "expired", "invalid", "ack-failed", "command-failed"} {
		t.Run(kind, func(t *testing.T) {
			resetRebootGuard(t)
			id := "test-request"
			deadline := time.Now().Add(30 * time.Second)
			if kind == "patching" {
				activePatchRuns = 1
			}
			if kind == "updating" {
				agentUpdating = true
			}
			if kind == "expired" {
				deadline = time.Now().Add(-time.Second)
			}
			if kind == "invalid" {
				id = "; shutdown"
			}
			calls := 0
			var statuses []string
			processReboot(id, deadline, func(r rebootReply) error {
				statuses = append(statuses, r.Status)
				if kind == "ack-failed" {
					return errors.New("offline")
				}
				return nil
			}, func() error { calls++; return errors.New("permission denied") }, func() {})
			if kind == "command-failed" {
				if calls != 1 || rebootPending || !reflect.DeepEqual(statuses, []string{"accepted", "failed"}) {
					t.Fatalf("%d %v pending=%v", calls, statuses, rebootPending)
				}
			} else if calls != 0 {
				t.Fatal("unsafe execution")
			}
		})
	}
}

func TestAgentOutputPreservesNBBuild(t *testing.T) {
	m := agentVersionOutputRe.FindStringSubmatch("patchmon-agent version 2.1.6~nb1")
	if len(m) < 2 || m[1] != "2.1.6~nb1" {
		t.Fatalf("%v", m)
	}
}
