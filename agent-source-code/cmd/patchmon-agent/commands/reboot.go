package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"time"
)

// Protected by patchUpdateMu, shared with patching and self-update guards.
var rebootPending bool
var rebootPendingID string

type rebootReply struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

var rebootRequestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

func rebootCommand(platform string) (string, []string, error) {
	switch platform {
	case "linux", "freebsd":
		return "shutdown", []string{"-r", "now"}, nil
	case "windows":
		return "shutdown.exe", []string{"/r", "/t", "0"}, nil
	default:
		return "", nil, errors.New("remote reboot is not supported on this platform")
	}
}

func executeHostReboot() error {
	name, args, err := rebootCommand(runtime.GOOS)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}

// processReboot is injectable so tests never execute shutdown. An acknowledgement
// precedes execution, allowing it to reach the server before the connection drops.
func processReboot(id string, expiresAt time.Time, send func(rebootReply) error, run func() error, pause func()) {
	reply := func(status, message string) error {
		return send(rebootReply{Type: "reboot_result", RequestID: id, Status: status, Message: message})
	}
	if !rebootRequestIDPattern.MatchString(id) || time.Now().After(expiresAt) || time.Until(expiresAt) > time.Minute {
		_ = reply("failed", "Invalid or expired reboot request")
		return
	}
	patchUpdateMu.Lock()
	if rebootPending && rebootPendingID == id {
		patchUpdateMu.Unlock()
		_ = reply("accepted", "Reboot request already accepted; waiting for the host to restart")
		return
	}
	if rebootPending || activePatchRuns > 0 || agentUpdating {
		patchUpdateMu.Unlock()
		_ = reply("failed", "Reboot refused: patching, an agent update, or a reboot is already in progress")
		return
	}
	rebootPending = true
	rebootPendingID = id
	patchUpdateMu.Unlock()
	releaseReboot := func() {
		patchUpdateMu.Lock()
		rebootPending = false
		rebootPendingID = ""
		patchUpdateMu.Unlock()
	}
	if err := reply("accepted", "Agent accepted the reboot request; waiting for the host to restart"); err != nil {
		releaseReboot() // Never reboot if the acknowledgement could not be sent.
		return
	}
	pause()
	if time.Now().After(expiresAt) {
		releaseReboot()
		_ = reply("failed", "Reboot request expired before execution")
		return
	}
	if err := run(); err != nil {
		releaseReboot()
		_ = reply("failed", fmt.Sprintf("Host reboot command failed: %v", err))
		return
	}
	// Remain reserved until the agent process exits: do not allow a second reboot
	// or a patch/update to race shutdown. A successful command is not proof of boot.
	_ = reply("requested", "Reboot command issued; waiting for a new boot time")
}

func handleHostReboot(m wsMsg) {
	if m.rebootConn == nil {
		return
	}
	processReboot(m.rebootRequestID, time.Unix(m.rebootExpiresAt, 0), func(reply rebootReply) error {
		payload, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		return writeWebSocketTextMessage(m.rebootConn, payload)
	}, executeHostReboot, func() { time.Sleep(2 * time.Second) })
}
