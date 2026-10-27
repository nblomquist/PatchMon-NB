# Manual host reboots and pending-update notices

## Goal

Keep a normal-sized **Reboot** button in the host detail header. Allow manual
reboots even when no restart is pending, provided the user has permission, the
agent is connected and supports the command, and no conflicting operation is
running. Require confirmation before sending the request.

Show a separate contextual banner when installed kernel or other updates require
a restart, with extra emphasis for confirmed security-related updates. Do not
put a second, larger reboot action in that banner. With no pending restart, omit
the banner but keep the header action available. See the
[revision 02 mockups](mockups/host-reboot/index.html).

This is a plan, not an implemented feature. No machines should be rebooted as
part of automated tests.

## Existing building blocks

- `agent-source-code/internal/system/reboot.go` detects pending reboots.
- Host reports already carry reboot status and a reason; the UI uses
  `needs_reboot` and `reboot_reason`.
- `frontend/src/components/HostStatusPills.jsx` displays reboot status.
- `frontend/src/pages/HostDetail.jsx` is the host detail page.
- `frontend/src/utils/api.js` contains host API calls.
- `server-source-code/internal/handler/hosts.go` handles host actions, including
  agent updates and direct commands over the agent connection.
- `server-source-code/internal/server/router.go` wires host routes and permission
  checks.
- `agent-source-code/cmd/patchmon-agent/commands/serve.go` receives and dispatches
  server commands over WebSocket.
- Agent-update code exists in
  `agent-source-code/cmd/patchmon-agent/commands/version_update.go`.

These provide a starting point. Remote reboot execution and the fork's binary
distribution path still need to be designed and verified.

## Work to do

### 1. Confirm agent build and update distribution

- [ ] Trace how agents discover, download, and verify updates.
- [ ] Inspect build/release workflows and how the server obtains agent binaries.
- [ ] Configure the deployment to distribute binaries built from this fork,
  rather than accidentally replacing them with upstream binaries.
- [ ] Decide how fork versions are identified and compared for updates.
- [ ] Define how the server detects agents that support the new reboot command.
  Older agents must not silently appear to accept unsupported requests.
- [ ] Document build, release, rollout, and rollback steps for the fork.

### 2. Add the server action

- [ ] Add a host reboot endpoint and a frontend API helper.
- [ ] Require host-management permission and preserve tenant/host isolation.
- [ ] Allow requests whether or not a reboot is pending. Reject requests when
  the agent is offline or does not support the command.
- [ ] Block reboot during active patching; review conflicts with agent updates
  and other host-changing work.
- [ ] Send a bounded, explicit command through the existing agent connection.
  Do not queue a reboot indefinitely for an offline host.
- [ ] Define acknowledgement, failure reporting, timeout, and duplicate-request
  behavior. Sending a command is not proof that the machine rebooted.
- [ ] Record who requested the reboot, the target host, and the request outcome
  using the project's audit conventions.

### 3. Implement the agent command

- [ ] Add command parsing and dispatch to the agent.
- [ ] Implement reboot execution for supported operating systems, including
  Linux, FreeBSD, and Windows, or explicitly reject unsupported platforms.
- [ ] Use fixed executable/argument lists, not user-provided shell commands.
- [ ] Check local operation guards so a reboot cannot interrupt patching or an
  agent update if server-side state is stale.
- [ ] Do not require a local pending-reboot flag for an operator-requested reboot.
  Treat detector/patch-run status as explanatory context, not permission to reboot.
- [ ] Report command acceptance or execution failure before the connection is
  lost, where possible. Define what acceptance actually guarantees.
- [ ] Prevent duplicate commands from scheduling repeated reboots.

### 4. Add the host-page controls

- [ ] Always show a compact Reboot header button to users with host-management
  permission, matching the existing toolbar button sizing and styling.
- [ ] Disable it only for offline/unsupported agents or conflicting operations
  and requests, not because no restart is pending. Explain disabled states.
- [ ] Show the reported reboot reason in a separate banner only when a restart
  is required. Distinguish kernel and other update reasons where known.
- [ ] Establish trustworthy security-update/reboot linkage before showing a
  Security label; a generic reboot flag alone is not evidence. Retain a generic
  warning when security impact is unknown.
- [ ] Require a confirmation dialog naming the host and warning about downtime.
- [ ] Show request progress and errors without claiming reboot completion just
  because the server sent the command.
- [ ] Refresh host status as it disconnects and reconnects. Let fresh reports
  clear pending reboot status; do not clear it optimistically.

### 5. Verify and document

- [ ] Frontend tests: manual reboot with no pending flag, contextual/security
  banner visibility, disabled states, permissions, confirmation, cancellation,
  duplicate clicks, and request failures.
- [ ] Server tests: permissions, host isolation, reboot with/without a pending flag,
  offline/unsupported agents, active patching, and delivery failures.
- [ ] Agent tests: parsing, operation guards, duplicate handling, platform command
  selection, and execution failures. Inject a fake command runner so tests never
  reboot the development or CI machine.
- [ ] Run the relevant Go tests for both server and agent, frontend tests, lint,
  and a frontend production build.
- [ ] On a disposable VM only, verify the full path: fork agent update, pending
  reboot indication, confirmed request, disconnect, changed boot time, reconnect,
  and cleared reboot status. Require explicit approval before this disruptive
  manual test.
- [ ] Document deployment requirements, supported platforms, and limitations.

## Rollout order

1. Establish and verify distribution of this fork's agent binaries.
2. Deploy agents that support reboot commands.
3. Deploy the server and UI changes, with compatibility checks for older agents.
4. Validate on a disposable host before enabling use on production machines.

## Scope boundaries

- Manual, per-host reboot only; no automatic or bulk reboots in this first change.
- No inbound connectivity requirement: use the existing outbound agent connection.
- Creating these notes does not publish a release, push a branch, update agents,
  or authorize rebooting any machine.
