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

The UI, server action, and agent command are implemented for build `~nb1`.
Rollout and a real reboot on an explicitly approved disposable machine remain
separate work. No machines should be rebooted as part of automated tests.

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

## Implementation evidence path

Implement an additive, expiring agent command with an agent acknowledgement and
Redis-backed request status/duplicate protection. Keep the existing outbound
connection and request-scoped access checks. Acceptance means the agent accepted
the request, not that a reboot completed; only a changed boot time establishes
completion. Execute fixed platform commands through an injectable runner, never
from request-provided shell text. Automated tests must only use fake runners.

Verify version parsing/comparison and binary metadata with `~nbN`, then agent
guards and command handling, server authorization/isolation and fake-agent
delivery, frontend controls/banner/confirmation/status, full project checks, and
a fresh isolated container/browser smoke test. No real reboot is authorized.
Security wording must fall back to the reported generic reason unless there is
explicit security evidence; a generic kernel mismatch is not sufficient.

## Build 1 behaviour

- Header Reboot action uses the same toolbar styling as Fetch Report and is
  available without a pending-reboot flag. A separate reason banner appears only
  when the host reports `needs_reboot`.
- Both routes (`GET` and `POST /api/v1/hosts/{hostId}/reboot`) require
  `can_manage_hosts`. POST also requires `{ "confirmed": true }`.
- Older/upstream agents are not sent reboot commands: this build gates the action
  on a reported NB agent version at least `2.1.6~nb1`. Upgrade agents first.
- The server checks the request's host scope, live agent connection, and active
  patch runs. Audit persistence is required before dispatch.
- Requests are atomically reserved in request-scoped Redis across server
  processes. They are not background jobs; each command expires after 30 seconds.
  Duplicate protection/status expires after ten minutes. If no new boot time is
  observed after two minutes, status becomes unknown and advises checking the
  machine before retrying; it does not claim success or schedule a later reboot.
- The agent shares a guard with patching and self-updates, rejects expired
  requests, acknowledges before execution, and treats a repeated accepted request
  ID as idempotent. If acknowledgement cannot be sent, it does not reboot.
- After a two-second acknowledgement grace period, Linux/FreeBSD use fixed
  `shutdown -r now` arguments and Windows uses `shutdown.exe /r /t 0`, without
  force-killing applications. The service account needs OS reboot privileges.
- Command errors are returned to the server. An issued command is still not
  proof of a completed reboot: a freshly reported boot time after the request
  establishes completion. Host clocks must be accurate for that comparison.
- The Security label is deliberately conservative: it requires an explicit
  positive security-update reason from the report. Generic kernel/flag-file
  reasons and security package counts do not establish that linkage and retain
  a generic notice. Automatic classification from patch metadata remains future
  work; this build does not guess.
- No schema migration, existing configuration change, or new dependency is
  required. Request/outcome audit entries use the existing security audit store.

## Remaining rollout and follow-up checklist

### 1. Confirm agent build and update distribution

- [ ] Trace how agents discover, download, and verify updates.
- [ ] Inspect build/release workflows and how the server obtains agent binaries.
- [ ] Configure the deployment to distribute binaries built from this fork,
  rather than accidentally replacing them with upstream binaries.
- [x] Decide how fork versions are identified and compared for updates.
- [x] Define how the server detects agents that support the new reboot command.
  Older agents must not silently appear to accept unsupported requests.
- [ ] Document build, release, rollout, and rollback steps for the fork.

### 2. Add the server action

- [x] Add a host reboot endpoint and a frontend API helper.
- [x] Require host-management permission and preserve tenant/host isolation.
- [x] Allow requests whether or not a reboot is pending. Reject requests when
  the agent is offline or does not support the command.
- [x] Block reboot during active patching and agent updates.
- [ ] Assess guards for additional host-changing operations, such as compliance
  remediation; those are not currently covered by the patch/update guard.
- [x] Send a bounded, explicit command through the existing agent connection.
  Do not queue a reboot indefinitely for an offline host.
- [x] Define acknowledgement, failure reporting, timeout, and duplicate-request
  behavior. Sending a command is not proof that the machine rebooted.
- [x] Record who requested the reboot, the target host, and the request outcome
  using the project's audit conventions.

### 3. Implement the agent command

- [x] Add command parsing and dispatch to the agent.
- [x] Implement reboot execution for supported operating systems, including
  Linux, FreeBSD, and Windows, or explicitly reject unsupported platforms.
- [x] Use fixed executable/argument lists, not user-provided shell commands.
- [x] Check local operation guards so a reboot cannot interrupt patching or an
  agent update if server-side state is stale.
- [x] Do not require a local pending-reboot flag for an operator-requested reboot.
  Treat detector/patch-run status as explanatory context, not permission to reboot.
- [x] Report command acceptance or execution failure before the connection is
  lost, where possible. Define what acceptance actually guarantees.
- [x] Prevent duplicate commands from scheduling repeated reboots.

### 4. Add the host-page controls

- [x] Always show a compact Reboot header button to users with host-management
  permission, matching the existing toolbar button sizing and styling.
- [x] Disable it for offline/unsupported agents, active patching, or conflicting
  and requests, not because no restart is pending. Explain disabled states.
- [x] Show the reported reboot reason in a separate banner only when a restart
  is required. Distinguish kernel and other update reasons where known.
- [x] Use only explicitly reported positive security-update context for the
  Security label; a generic reboot flag alone is not evidence. Retain a generic
  warning when security impact is unknown.
- [ ] Add structured security-update/reboot linkage beyond explicit reason text.
- [x] Require a confirmation dialog naming the host and warning about downtime.
- [x] Show request progress and errors without claiming reboot completion just
  because the server sent the command.
- [x] Refresh host status as it disconnects and reconnects. Let fresh reports
  clear pending reboot status; do not clear it optimistically.

### 5. Verify and document

- [x] Frontend tests: manual reboot with no pending flag, contextual/security
  banner visibility, disabled states, permissions, confirmation, cancellation,
  duplicate clicks, and request failures.
- [x] Server tests: permissions, host isolation, reboot without a pending flag,
  offline/unsupported agents, active patching, and delivery failures.
- [x] Agent tests: request validation, operation guards, duplicate handling, platform command
  selection, and execution failures. Inject a fake command runner so tests never
  reboot the development or CI machine.
- [x] Run the relevant Go tests for both server and agent, frontend tests, lint,
  and a frontend production build.
- [ ] On a disposable VM only, verify the full path: fork agent update, pending
  reboot indication, confirmed request, disconnect, changed boot time, reconnect,
  and cleared reboot status. Require explicit approval before this disruptive
  manual test.
- [x] Document deployment requirements, supported platforms, and limitations.

## Verification: 27 October 2026

- Both Go module `make check` targets, Biome, frontend tests, production npm audit
  threshold, and both govulncheck scans passed via
  `mise exec -- npx lefthook run pre-commit --all-files`, with
  `PATCHMON_TEST_DATABASE_URL` pointing at a disposable PostgreSQL 17 instance.
  Frontend: 45 test files, 601 tests passed, three existing skips. Database-backed
  server tests, including reboot permission checks, ran rather than skipping.
- Reboot/version server tests also passed with `go test -race`; agent shutdown
  tests use fake runners only. Windows/FreeBSD commands were selected and
  cross-compiled, not executed on live hosts.
- `mise exec -- ./docker/build.sh --tag nb1` built all ten agents and Linux
  amd64/arm64 servers. Final local image ID:
  `sha256:69bff993e02d890f5cee0e862d839b848284f61eb55d5c480d48d4250cc30a8c`.
- All services in the isolated Compose stack became healthy. Authenticated
  version API requests for all ten agent targets returned `2.1.6~nb1`; executing
  only the native agent's `--version` command returned that same version.
- Playwright CLI exercised the actual built UI with an authenticated WebSocket
  simulator incapable of running OS commands: normal-sized manual reboot with
  no pending flag, cancel/confirm, HTTP 202, accepted/requested status, duplicate
  disabling, and completion after a **simulated** fresh boot-time report.
- The final image recorded the four expected events: `host_reboot_requested`,
  `host_reboot_accepted`, `host_reboot_command_issued`, `host_reboot_completed`.
  Browser checks also verified explicit security wording, the agreed offline
  text, disabled offline controls, a 38px toolbar button matching Fetch Report,
  displayed server/agent NB versions, and no horizontal overflow at 390px.
- No real reboot, installed-agent self-update, registry publication, or production
  deployment was performed. Actual OS restart/reconnect behaviour still needs
  explicit approval for testing on a disposable VM.

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
