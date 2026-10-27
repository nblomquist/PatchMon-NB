# Patch Selected hosts

Status: design proposal; application code is unchanged. Branch: `nb/bulk-host-patching`.

## Intent and wording

Select an explicit subset on Hosts, then patch either all updates or only reported
security updates. Use **Patch Selected** with a dropdown chevron. Menu choices:
**All updates** and **Security updates only**. Neither the button nor a menu choice
executes updates immediately; both enter the existing patch wizard.

[Interactive mockup](mockups/bulk-host-patching/index.html) uses synthetic data,
has no network calls, and cannot patch a machine. Review it before implementation.

## Proposed experience

- Put the compact button in the existing selection toolbar beside Fetch Reports.
  Show it when at least one host is selected and patch permission is present.
- Clicking Patch Selected or its chevron opens the same menu; no ambiguous
  one-click execution half of a split button. All updates is first, not a sticky
  choice inherited from a previous operation.
- Take an explicit snapshot of selected IDs when opening the wizard. Do not
  expand it to a filter, group, page, or all hosts. Existing selection across
  pages remains supported; show the full selected list, including hidden rows.
- Reuse the canonical Hosts / Packages / Validate / Timing / Approval / Submit
  wizard, skipping only the steps it already skips where appropriate. Keep host
  scope fixed, with names available for review. No separate fast-patching path.
- Show ready, skipped, and blocked hosts individually and explain why. Refresh
  eligibility before dispatch; stale reports, permissions, connection changes,
  active runs, reboots, and agent updates must not be hidden behind a count.
- Never silently drop a host. Explicitly confirm the ready subset, or return to
  the list. Disable submission if no hosts are eligible.
- Preserve existing patch policy, timing, approval, and reboot settings. Do not
  force immediate execution or introduce automatic reboots. Explain that services
  may restart and policy-controlled reboot behaviour still applies.
- Report per-host submission results with links to Runs & History. Created,
  scheduled, awaiting approval, running, and completed are different states.
  Mixed results must not be reported as universal success. Retry only failed
  submissions, after refreshing state; do not resubmit successful hosts.
- Mobile: wrap existing toolbar actions; keep menu and wizard within the viewport.
  Support keyboard activation, Escape, focus return, and labelled menu choices.

## Repository findings and implementation approach

- `frontend/src/pages/Hosts.jsx` already owns selection and the bulk toolbar.
- `frontend/src/components/PatchWizard.jsx` accepts `presetHosts`, `lockHosts`,
  and `patchType`, and already limits concurrent requests. Prefer extending this
  flow over adding a bulk endpoint or another execution engine.
- `frontend/src/utils/patchingApi.js` and the existing server Trigger/ApproveRun
  routes own per-host runs. Preserve their tenant scope, permissions, active-run
  exclusion, audit entries, and scheduling behaviour.
- All updates should use existing `patch_all`, preserving OS-specific scope
  choices, including Windows updates versus applications. Confirm that the shared
  wizard renders scope correctly for mixed Linux/FreeBSD/Windows selections.
- There is no security-only field in the current Trigger request. Do not send an
  ignored boolean or label `patch_all` as security-only.
- Security-only needs a per-host list of eligible, installed, outdated packages
  or Windows update identities with affirmative security metadata. Names and
  counts alone are not enough. Never union security package names across hosts.
  Resolve and recheck scope server-side before execution/approval. Extend the
  wizard narrowly for per-host package selections if needed.
- Package managers may install dependencies as part of a security update. Say
  “Security updates and required dependencies”; do not promise that no other
  package changes. Unknown security classification is not equivalent to zero
  security updates. Unsupported classification must block that host in this mode.
- `patchSupport.js` and server `selectedPatchRefusal` prohibit selected-package
  upgrades on pacman. Security-only must remain unavailable there; never substitute
  a full upgrade. Determine reliable security metadata support for each other
  OS/package-manager combination before enabling it.
- Avoid an agent protocol change unless existing selected-package commands cannot
  express the approved scope safely. Any required extension must gate capability
  for older agents and document mixed-version behaviour.

## Evidence path

Claim: only explicitly confirmed eligible hosts receive the chosen update scope,
with existing policy and permissions intact; partial failures remain visible.

1. Frontend tests: empty/single/multi selection, menu wording and keyboard flow,
   cross-page selection, no selection expansion, cancel with zero requests,
   fixed host scope, both modes, mixed OS scope, blocked/all-blocked hosts,
   repeat clicks, partial failures, retry scope, and timing/approval retention.
2. Server tests with disposable PostgreSQL: host/tenant permissions, independent
   per-host runs, active-run conflicts, exact security lists for hosts with
   different packages, unknown/stale classification, unsupported platforms,
   revalidation at approval, and no fallback to patch_all.
3. Fake agent runners: assert exact existing payloads, operation guards, security
   dependency wording, and compatibility. Never install packages on the dev host.
4. Browser smoke: actual built UI against safe fixtures/simulated agents; inspect
   host IDs, request scopes, run records, and mixed outcomes. Desktop and 390px
   mobile screenshots. Mockup checks alone do not establish application behaviour.
5. Run both Go make check targets, frontend tests, Biome, vulnerability gates,
   and relevant race tests. Build bundled agents only if their code changes.

Fallback: if security-only cannot be established safely for a platform, show an
explicit unsupported reason. Broader proof requires separately approved disposable
VM tests; production patching is not part of design or automated verification.

## Delivery checklist

- [x] Create branch and inspect selection/patching entrypoints.
- [x] Document proposed behaviour and evidence path.
- [x] Create interactive desktop/mobile design mockup.
- [x] Browser-check mockup interactions and capture desktop/mobile states.
- [ ] Review mockups and approve interaction and skip rules.
- [ ] Establish supported security classification and per-host scope handling.
- [ ] Implement all-updates flow through the existing wizard.
- [ ] Implement security-only without widening scope.
- [ ] Verify tests, browser flow, and mixed outcomes; update these notes.
- [ ] Increment NB_BUILD before distributing the changed build.

No commits, image publication, deployment, or real patching are implied by this plan.
