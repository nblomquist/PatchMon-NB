package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	redis "github.com/redis/go-redis/v9"
)

type rebootHostStore interface {
	GetByID(context.Context, string) (*models.Host, error)
	HasActivePatchRun(context.Context, string) (bool, error)
}

type rebootAudit interface {
	Record(context.Context, store.AuditEntry) error
}

// Request state is shared across server processes. It is never a deferred job:
// commands expire in 30s, and only a currently connected agent can receive one.
type HostRebootHandler struct {
	hosts    rebootHostStore
	registry *agentregistry.Registry
	redis    *hostctx.RedisResolver
	audit    rebootAudit
}

func NewHostRebootHandler(hosts *store.HostsStore, registry *agentregistry.Registry, rdb *hostctx.RedisResolver, audit *store.AuditLogsStore) *HostRebootHandler {
	return &HostRebootHandler{hosts: hosts, registry: registry, redis: rdb, audit: audit}
}

type rebootRequest struct {
	ID        string     `json:"request_id"`
	Status    string     `json:"status"`
	Message   string     `json:"message"`
	BootTime  *time.Time `json:"boot_time,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ActorID   string     `json:"actor_id,omitempty"`
}

var nbAgentVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?~nb[1-9][0-9]*$`)

func supportsHostReboot(version *string) bool {
	return version != nil && nbAgentVersion.MatchString(*version) && util.CompareVersions(*version, "2.1.6~nb1") >= 0
}

func rebootKey(ctx context.Context, apiID string) string {
	return fmt.Sprintf("nb:reboot:%x", sha256.Sum256([]byte(hostctx.TenantHostKey(ctx)+"\x00"+apiID)))
}

const rebootTTL = 10 * time.Minute

const acquireReboot = `local current = redis.call('GET', KEYS[1])
if current then
  local s = cjson.decode(current).status
  if s ~= 'failed' and s ~= 'completed' then return 0 end
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2]); return 1`

const replaceReboot = `local current = redis.call('GET', KEYS[1])
if not current or cjson.decode(current).request_id ~= ARGV[1] then return 0 end
if cjson.decode(current).status ~= ARGV[2] then return 0 end
redis.call('SET', KEYS[1], ARGV[3], 'KEEPTTL'); return 1`

func (h *HostRebootHandler) load(ctx context.Context, apiID string) (*rebootRequest, error) {
	raw, err := h.redis.RDB(ctx).Get(ctx, rebootKey(ctx, apiID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state rebootRequest
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (h *HostRebootHandler) change(ctx context.Context, apiID string, state *rebootRequest, status, message string) error {
	previous := state.Status
	state.Status, state.Message = status, message
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	changed, err := h.redis.RDB(ctx).Eval(ctx, replaceReboot, []string{rebootKey(ctx, apiID)}, state.ID, previous, raw).Int()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("reboot request changed concurrently")
	}
	return nil
}

func (h *HostRebootHandler) stateForHost(ctx context.Context, host *models.Host) (*rebootRequest, error) {
	state, err := h.load(ctx, host.ApiID)
	if err != nil || state == nil {
		return state, err
	}
	if state.Status == "failed" || state.Status == "completed" {
		return state, nil
	}
	if host.BootTime != nil && host.BootTime.After(state.CreatedAt) && (state.BootTime == nil || host.BootTime.After(*state.BootTime)) {
		err = h.change(ctx, host.ApiID, state, "completed", "A new boot time was reported by the host")
		if err == nil && h.audit != nil {
			if auditErr := h.audit.Record(ctx, store.AuditEntry{Event: "host_reboot_completed", ActorUserID: state.ActorID, Success: true, Metadata: map[string]any{"host_id": host.ID, "request_id": state.ID}}); auditErr != nil {
				slog.Error("reboot completion audit failed", "error", auditErr)
			}
		}
	} else if time.Since(state.CreatedAt) > 2*time.Minute {
		err = h.change(ctx, host.ApiID, state, "unknown", "No new boot time confirmed. Check the host before retrying.")
	}
	return state, err
}

func (h *HostRebootHandler) Status(w http.ResponseWriter, r *http.Request) {
	host, err := h.hosts.GetByID(r.Context(), chi.URLParam(r, "hostId"))
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	state, err := h.stateForHost(r.Context(), host)
	if err != nil {
		Error(w, http.StatusServiceUnavailable, "Reboot status unavailable")
		return
	}
	active, err := h.hosts.HasActivePatchRun(r.Context(), host.ID)
	if err != nil {
		Error(w, http.StatusServiceUnavailable, "Unable to check patch activity")
		return
	}
	JSON(w, http.StatusOK, map[string]any{"supported": supportsHostReboot(host.AgentVersion), "patching": active, "request": state})
}

func (h *HostRebootHandler) Reboot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := ctx.Value(middleware.UserIDKey).(string)
	if actor == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := decodeJSON(r, &body); err != nil || !body.Confirmed {
		Error(w, http.StatusBadRequest, "Explicit reboot confirmation is required")
		return
	}
	host, err := h.hosts.GetByID(ctx, chi.URLParam(r, "hostId"))
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	if !supportsHostReboot(host.AgentVersion) {
		Error(w, http.StatusConflict, "Update this agent to an NB build supporting reboot first")
		return
	}
	if h.registry == nil || !h.registry.IsConnected(host.ApiID) || h.registry.Get(host.ApiID).Scope != hostctx.TenantHostKey(ctx) {
		Error(w, http.StatusConflict, "Agent offline — waiting for the agent to reconnect")
		return
	}
	active, err := h.hosts.HasActivePatchRun(ctx, host.ID)
	if err != nil {
		Error(w, http.StatusServiceUnavailable, "Unable to check patch activity")
		return
	}
	if active {
		Error(w, http.StatusConflict, store.PatchRunInProgressMessage)
		return
	}
	if _, err := h.stateForHost(ctx, host); err != nil {
		Error(w, http.StatusServiceUnavailable, "Reboot service unavailable")
		return
	}
	state := &rebootRequest{ID: uuid.NewString(), Status: "pending", Message: "Request sent; waiting for agent acknowledgement", BootTime: host.BootTime, CreatedAt: time.Now().UTC(), ActorID: actor}
	raw, _ := json.Marshal(state)
	created, err := h.redis.RDB(ctx).Eval(ctx, acquireReboot, []string{rebootKey(ctx, host.ApiID)}, raw, int(rebootTTL.Seconds())).Int()
	if err != nil {
		Error(w, http.StatusServiceUnavailable, "Reboot service unavailable")
		return
	}
	if created == 0 {
		Error(w, http.StatusConflict, "A reboot request is already pending; check the host before retrying")
		return
	}
	if h.audit == nil || h.audit.Record(ctx, store.AuditEntry{Event: "host_reboot_requested", ActorUserID: actor, IPAddress: r.RemoteAddr, Success: true, Metadata: map[string]any{"host_id": host.ID, "request_id": state.ID}}) != nil {
		_ = h.change(ctx, host.ApiID, state, "failed", "Audit record could not be written; no command sent")
		Error(w, http.StatusInternalServerError, "Cannot reboot without an audit record")
		return
	}
	command, _ := json.Marshal(map[string]any{"type": "reboot_host", "request_id": state.ID, "expires_at": time.Now().Add(30 * time.Second).Unix()})
	if err := h.registry.SendMessageWithTimeout(host.ApiID, websocket.TextMessage, command, 5*time.Second); err != nil {
		// A transport failure can occur after delivery. Do not advertise safe retry.
		_ = h.change(ctx, host.ApiID, state, "unknown", "Delivery was not confirmed. Check the host before retrying.")
		Error(w, http.StatusServiceUnavailable, "Reboot delivery was not confirmed; check request status before retrying")
		return
	}
	JSON(w, http.StatusAccepted, state)
}

// HandleResult is invoked only for messages on an authenticated agent connection.
func (h *HostRebootHandler) HandleResult(ctx context.Context, apiID string, message []byte) {
	var reply struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(message, &reply) != nil || len(reply.Message) > 1000 {
		return
	}
	if reply.Status != "accepted" && reply.Status != "requested" && reply.Status != "failed" {
		return
	}
	state, err := h.load(ctx, apiID)
	if err != nil || state == nil || state.ID != reply.RequestID || state.Status == "failed" || state.Status == "completed" {
		return
	}
	if state.Status == "requested" && reply.Status == "accepted" {
		return
	}
	if err := h.change(ctx, apiID, state, reply.Status, reply.Message); err != nil {
		slog.Error("reboot result persistence failed", "error", err)
		return
	}
	if h.audit != nil {
		event := "host_reboot_" + reply.Status
		if reply.Status == "requested" {
			event = "host_reboot_command_issued"
		}
		if err := h.audit.Record(ctx, store.AuditEntry{Event: event, ActorUserID: state.ActorID, Success: reply.Status != "failed", Metadata: map[string]any{"api_id": apiID, "request_id": state.ID, "message": reply.Message}}); err != nil {
			slog.Error("reboot outcome audit failed", "error", err)
		}
	}
}
