# Patch reports during short connection outages

## Failure and fix

A package upgrade can finish while PatchMon's public proxy is restarting. In the
October 27 incident, updating Docker on the Pangolin host stopped Traefik,
Pangolin and Gerbil. Agents saw connection refusals followed by HTTP 404/502
responses. Package operations completed, but their terminal reports did not
reach PatchMon.

The Unix agent previously returned immediately when completion delivery failed,
also skipping its post-patch inventory refresh. Windows already attempted that
refresh independently, but neither path retried final reports through HTTP
errors from a recovering proxy.

Both paths now retry the same final report every five seconds after a failed
attempt, within the existing two-minute final-delivery deadline. Existing HTTP
transport retries still apply inside an attempt. No package operation is rerun.
Inventory refresh is attempted after delivery succeeds or its deadline expires,
using its own existing timeout. Dry runs still do not trigger inventory refresh;
the existing platform-specific rules for failed/cancelled runs are unchanged.

## Compatibility and limits

- Agent-only change; no API, database, configuration or frontend changes.
- Final-report retry uses the existing stage and full-output payload.
- No disk queue: reports do not survive agent restart or an outage longer than
  the final-delivery window. Scheduled inventory check-ins remain the fallback
  when the immediate inventory refresh also fails.
- Existing server terminal-state guards remain intact: late completion can
  recover `timed_out`/`agent_disconnected` runs, but cannot overwrite a cancelled
  run. This change does not replay reports for historical runs.
- Windows per-update item delivery is unchanged; this retries the final report,
  not missing item reports.
- Rebuild and update installed agents to deploy. Increment `NB_BUILD` before
  distributing a changed fork build; this branch does not deploy anything.

## Verification

`TestFinishPatchReportProxyRecovery` sends real HTTP requests to a local test
server returning 404, then 502, then 200. It checks that every terminal stage
retries an unchanged payload and refreshes inventory once. Other helper tests
check cancellation with inventory refresh and immediate success without retry.
Existing Windows final-report tests cover the integrated call path. No test
performs a real package upgrade or restarts a service.

Run inside `agent-source-code/`:

```sh
mise exec -- make check
mise exec -- go test -race ./cmd/patchmon-agent/commands -run 'Test(FinishPatchReport|WindowsFinal|WindowsCompleted)' -count=1
```
