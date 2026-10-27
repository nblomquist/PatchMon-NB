package commands

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"patchmon-agent/internal/client"
)

func TestFinishPatchReportProxyRecovery(t *testing.T) {
	for _, stage := range []string{"completed", "failed", "cancelled", "dry_run_completed"} {
		t.Run(stage, func(t *testing.T) {
			fakeOutputServer(t) // Isolate credentials/config without touching host services.
			var bodies []map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/runs/run-1/output") {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				bodies = append(bodies, body)
				switch len(bodies) {
				case 1:
					w.WriteHeader(http.StatusNotFound)
				case 2:
					w.WriteHeader(http.StatusBadGateway)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer srv.Close()
			cfgManager.GetConfig().PatchmonServer = srv.URL
			httpClient := client.New(cfgManager, logger)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			refreshes := 0
			err := finishPatchReport(ctx, func(ctx context.Context) error {
				return httpClient.SendPatchOutput(ctx, "run-1", stage, "authoritative output", "result detail")
			}, func() { refreshes++ }, time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if len(bodies) != 3 || refreshes != 1 {
				t.Fatalf("posts=%d, refreshes=%d", len(bodies), refreshes)
			}
			want := map[string]string{"stage": stage, "output": "authoritative output", "error_message": "result detail"}
			for _, body := range bodies {
				if !reflect.DeepEqual(body, want) {
					t.Fatalf("retried payload changed: %+v", body)
				}
			}
		})
	}
}

func TestFinishPatchReportRefreshesAfterDeliveryDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wantErr := errors.New("proxy unavailable")
	attempts, refreshes := 0, 0
	err := finishPatchReport(ctx, func(context.Context) error {
		attempts++
		cancel()
		return wantErr
	}, func() { refreshes++ }, time.Hour)
	if !errors.Is(err, wantErr) || attempts != 1 || refreshes != 1 {
		t.Fatalf("err=%v, attempts=%d, refreshes=%d", err, attempts, refreshes)
	}
}

func TestFinishPatchReportSuccessDoesNotRetry(t *testing.T) {
	attempts := 0
	err := finishPatchReport(context.Background(), func(context.Context) error {
		attempts++
		return nil
	}, nil, time.Hour)
	if err != nil || attempts != 1 {
		t.Fatalf("err=%v, attempts=%d", err, attempts)
	}
}
