package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	redis "github.com/redis/go-redis/v9"
)

type fakeRebootHosts struct {
	host   *models.Host
	active bool
	err    error
}

func (s *fakeRebootHosts) GetByID(_ context.Context, id string) (*models.Host, error) {
	if s.host.ID != id {
		return nil, nil
	}
	return s.host, nil
}
func (s *fakeRebootHosts) HasActivePatchRun(context.Context, string) (bool, error) {
	return s.active, s.err
}

type fakeRebootAudit struct {
	entries []store.AuditEntry
	err     error
}

func (s *fakeRebootAudit) Record(_ context.Context, e store.AuditEntry) error {
	s.entries = append(s.entries, e)
	return s.err
}

func rebootFixture(t *testing.T) (*HostRebootHandler, *fakeRebootHosts, *fakeRebootAudit, *websocket.Conn) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	version := "2.1.6~nb1"
	boot := time.Now().Add(-time.Hour)
	hosts := &fakeRebootHosts{host: &models.Host{ID: "host-1", ApiID: "api-1", AgentVersion: &version, BootTime: &boot}}
	audit := &fakeRebootAudit{}
	registry := agentregistry.New()
	connections := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{}
		c, err := up.Upgrade(w, r, nil)
		if err == nil {
			connections <- c
		}
	}))
	t.Cleanup(srv.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server := <-connections
	t.Cleanup(func() { _ = server.Close() })
	registry.Register("api-1", false, "")
	registry.SetConnection("api-1", server)
	return &HostRebootHandler{hosts: hosts, registry: registry, redis: &hostctx.RedisResolver{Default: rdb}, audit: audit}, hosts, audit, client
}

func rebootHTTP(h *HostRebootHandler, method, id, body, actor string, ctx context.Context) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/hosts/"+id+"/reboot", strings.NewReader(body))
	route := chi.NewRouteContext()
	route.URLParams.Add("hostId", id)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, route)
	ctx = context.WithValue(ctx, middleware.UserIDKey, actor)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	if method == http.MethodPost {
		h.Reboot(w, r)
	} else {
		h.Status(w, r)
	}
	return w
}

func TestHostRebootDeliveryLifecycleAndDuplicateGuard(t *testing.T) {
	h, hosts, audit, client := rebootFixture(t)
	ctx := context.Background()
	w := rebootHTTP(h, "POST", "host-1", `{"confirmed":true}`, "operator", ctx)
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var command struct {
		Type, RequestID string
		ExpiresAt       int64
	}
	var wire map[string]any
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if err := client.ReadJSON(&wire); err != nil {
		t.Fatal(err)
	}
	command.Type, _ = wire["type"].(string)
	command.RequestID, _ = wire["request_id"].(string)
	command.ExpiresAt = int64(wire["expires_at"].(float64))
	if command.Type != "reboot_host" || command.RequestID == "" || time.Until(time.Unix(command.ExpiresAt, 0)) > 31*time.Second {
		t.Fatalf("%+v", command)
	}
	if hosts.host.NeedsReboot != nil && *hosts.host.NeedsReboot {
		t.Fatal("fixture must not require reboot")
	}
	if got := rebootHTTP(h, "POST", "host-1", `{"confirmed":true}`, "operator", ctx); got.Code != 409 {
		t.Fatal(got.Code)
	}
	for _, status := range []string{"accepted", "requested"} {
		msg, _ := json.Marshal(map[string]string{"request_id": command.RequestID, "status": status, "message": "test " + status})
		h.HandleResult(ctx, "api-1", msg)
		state, err := h.load(ctx, "api-1")
		if err != nil || state.Status != status {
			t.Fatalf("%+v %v", state, err)
		}
	}
	newBoot := time.Now()
	hosts.host.BootTime = &newBoot
	w = rebootHTTP(h, "GET", "host-1", "", "operator", ctx)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"completed"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(audit.entries) != 4 || audit.entries[0].ActorUserID != "operator" {
		t.Fatalf("audit: %+v", audit.entries)
	}
}

func TestHostRebootRefusals(t *testing.T) {
	for _, kind := range []string{"unconfirmed", "anonymous", "offline", "upstream", "patching", "check-failed", "wrong-scope", "missing-host", "audit-failed"} {
		t.Run(kind, func(t *testing.T) {
			h, hosts, audit, _ := rebootFixture(t)
			ctx := context.Background()
			body := `{"confirmed":true}`
			actor := "operator"
			id := "host-1"
			want := 409
			switch kind {
			case "unconfirmed":
				body = `{}`
				want = 400
			case "anonymous":
				actor = ""
				want = 401
			case "offline":
				h.registry = agentregistry.New()
			case "upstream":
				v := "2.2.0"
				hosts.host.AgentVersion = &v
			case "patching":
				hosts.active = true
			case "check-failed":
				hosts.err = errors.New("DB down")
				want = 503
			case "wrong-scope":
				ctx = hostctx.WithEntry(ctx, &hostctx.Entry{Host: "other.example"})
			case "missing-host":
				id = "missing"
				want = 404
			case "audit-failed":
				audit.err = errors.New("audit down")
				want = 500
			}
			w := rebootHTTP(h, "POST", id, body, actor, ctx)
			if w.Code != want {
				t.Fatalf("%d %s want %d", w.Code, w.Body.String(), want)
			}
		})
	}
}

func TestRebootResultIsolationAndFailure(t *testing.T) {
	h, _, _, _ := rebootFixture(t)
	ctx := context.Background()
	w := rebootHTTP(h, "POST", "host-1", `{"confirmed":true}`, "operator", ctx)
	var created rebootRequest
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	for _, tc := range []struct {
		ctx     context.Context
		api, id string
	}{
		{ctx, "other-agent", created.ID}, {ctx, "api-1", "wrong-id"}, {hostctx.WithEntry(ctx, &hostctx.Entry{Host: "other.example"}), "api-1", created.ID},
	} {
		msg, _ := json.Marshal(map[string]string{"request_id": tc.id, "status": "failed", "message": "wrong"})
		h.HandleResult(tc.ctx, tc.api, msg)
	}
	state, _ := h.load(ctx, "api-1")
	if state.Status != "pending" {
		t.Fatal(state.Status)
	}
	msg, _ := json.Marshal(map[string]string{"request_id": created.ID, "status": "failed", "message": "permission denied"})
	h.HandleResult(ctx, "api-1", msg)
	state, _ = h.load(ctx, "api-1")
	if state.Status != "failed" {
		t.Fatal(state.Status)
	}
}

func TestHostRebootPermissionMiddleware(t *testing.T) {
	db := pgtest.Open(t)
	permissions := store.NewPermissionsStore(db)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(204) })
	for _, role := range []string{"", "user"} {
		r := httptest.NewRequest("POST", "/hosts/host-1/reboot", nil)
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserRoleKey, role))
		w := httptest.NewRecorder()
		middleware.RequirePermission("can_manage_hosts", permissions)(next).ServeHTTP(w, r)
		if called || w.Code < 400 {
			t.Fatalf("role=%q status=%d", role, w.Code)
		}
	}
}
