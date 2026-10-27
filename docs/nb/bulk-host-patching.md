# Patch Selected hosts

Status: implemented and locally verified on `nb/bulk-host-patching`.
The design was approved for implementation after mockup review.

## Intent and wording

Select an explicit subset on Hosts, then patch either all updates or only reported
security updates. Use **Patch Selected** with a dropdown chevron. Menu choices:
**All updates** and **Security updates only**. Neither the button nor a menu choice
executes updates immediately; both enter the existing patch wizard.

[Interactive mockup](mockups/bulk-host-patching/index.html) uses synthetic data,
has no network calls, and cannot patch a machine. It records the pre-implementation
design; the application now uses the existing wizard after a host eligibility review.

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
- [x] Review mockups and approve interaction and skip rules.
- [x] Establish supported security classification and per-host scope handling.
- [x] Implement all-updates flow through the existing wizard.
- [x] Implement security-only without widening scope.
- [x] Verify tests, browser flow, and mixed outcomes; update these notes.
- [x] Increment NB_BUILD to 2 for the next changed build; no registry upload implied.

## Implemented behaviour and limits

- The toolbar action requires `can_manage_patching` and the `patching` module.
  The new `POST /api/v1/patching/selection-preview` route enforces both too.
  It accepts 1–200 explicit host IDs, deduplicates them, and resolves off-page
  hosts in the request's database context. Filters never expand the selection.
- The review lists all selected hosts, including missing, offline, identity-conflicted,
  busy, rebooting, and unsupported hosts. The operator explicitly confirms the
  ready subset before entering the wizard, then confirms again on Submit.
- All updates uses the existing `patch_all` protocol. Hosts with no reported
  updates are skipped with a suggestion to fetch a report. Mixed-OS selections
  expose Windows Updates / Applications scope at Timing; it only affects Windows.
- Security-only initially supports apt, dnf/yum, zypper, and apk. It requires
  `host_packages.needs_update` **and** `is_security_update` on each target host,
  with each flagged row's `last_checked` no older than 24 hours and not more than
  five minutes in the future. Heartbeats do not refresh that timestamp.
- No affirmative classification means “No reported security updates; classification
  may be unavailable,” not a claim that the machine has no security exposure.
  Alpine secdb and repository advisory availability still affect agent reports.
- Security-only is blocked on Arch/pacman (partial upgrades are unsupported),
  FreeBSD (older agents mark base updates generically security-related), Windows
  (separate update-identity classification still needs a verified path), and
  unknown package managers. All updates remains available where upstream supports it.
- A maximum of 100 security package names per host matches the existing API.
  Larger selections are explicitly blocked rather than truncated or split into
  competing active runs. Package dependencies may also change.
- Requests use additive `selection_mode: "all" | "security"` on the existing
  trigger endpoint. Security mode must use `patch_package` plus explicit
  `package_names`; it rejects `patch_all`, empty lists, and packages not currently
  classified as security updates on that host. No new agent command is required.
- `nb_selection_mode` in the existing policy snapshot carries this constraint
  through validation, pending approval, approval, and scheduled execution.
  Approval and queue dispatch recheck inventory. Dispatch uses the persisted
  package list rather than widening it to new updates or trusting queue payload names.
- Existing timing, approval, operation guards, per-run records, and Windows
  identifiers remain in use. The agent's patch/update/reboot guard is the final
  exclusion check; the server cannot know all local in-progress agent work.
- A host disconnected at the initial review is skipped, not scheduled. A ready
  host can disconnect later; existing scheduled/offline queue behaviour still
  applies after explicit submission. Security inventory must still be fresh when
  that queued work reaches dispatch. This feature does not change reboot policy.
- Failed submissions stay in the wizard; retries only resubmit failures. Successful
  runs are reported even when the wizard is closed after a mixed result. Links to
  each run and Runs & History appear after closing. Created is not completed.
- No new database columns, migrations, dependencies, or agent protocol changes.
  No real package installations or upgrades are allowed during these checks.

## Verification · 27 October 2026

- Full enabled Lefthook checks passed with disposable PostgreSQL: both Go modules,
  frontend tests, Biome, production npm audit threshold, and govulncheck.
  Final frontend run: 46 files, 616 passing tests, three existing skips.
  Existing moderate npm advisories remain; no dependencies were changed.
- Focused race-enabled server tests passed for exact per-host security inventory,
  stale/unsupported classification, trigger refusal, approval reclassification,
  and fail-closed queue dispatch. Existing frontend wizard tests cover partial
  failure and successful-host retry exclusion.
- New frontend tests cover explicit off-page IDs, a selection snapshot unaffected
  by later parent changes, menu keyboard flow, empty/all-blocked selection,
  cancelled and mismatched previews, final confirmation, distinct package lists,
  no full-upgrade/union fallback, and mixed Windows scope handling.
- `mise exec -- ./docker/build.sh --tag bulk-test` built the production frontend,
  both Linux server architectures, and all ten bundled agent binaries as
  `2.1.6~nb2`. The local image is `patchmon-nb:bulk-test`.
- An isolated Compose stack became healthy with the implementation image. The
  actual built UI and authenticated API created two **pending approval** all-update
  runs with no execution; the recorded patch types were both `patch_all`.
- The browser then submitted security-only runs to two harmless authenticated
  WebSocket simulators. One host had a synthetic openssl security package, the
  other a synthetic sudo security package; each also had ordinary updates. Each
  simulator asserted it received **only its own security package** through the
  existing `run_patch` protocol. Both deliberately reported failure after recording
  the payload; neither had any OS command or package-manager execution capability.
  Database records retained the exact per-host package lists and security marker.
- Browser checks covered both confirmation gates, the ready-host review, policy
  and approval screens, final friendly-name run links, offline/all-blocked review,
  and 390px mobile review/wizard layouts with no page overflow. Desktop and mobile
  implementation captures were saved in the temporary local test workspace, not
  substituted for the pre-implementation mockups.
- Real package installations, production agent execution, OS-specific dependency
  resolution and production rollout were not tested or
  performed. These require separately approved disposable hosts. The existing
  published `:nb1` image is unchanged.

## Published test image

At the user's request, the tested Linux amd64 image was published to the existing
private GHCR package as `ghcr.io/nblomquist/patchmon-nb:nb2`. Registry inspection
confirmed the tag and package visibility remains **private**. Digest:
`sha256:d6f40840f899c01b2d2fdc9685b4e51430c892d176a8037a97b778498cf586d4`.
Dockhand can use the same GHCR credentials as NB1. The NB1 tag was not replaced.
Source implementation is on `nb/bulk-host-patching`; image publication did not
push or merge the source branch.

## Rollout and rollback

No agent protocol upgrade is required for this feature; it uses existing full and
selected-package patch commands. Server and bundled-agent versions will both use
`~nb2` when distributed, but installed NB1/upstream agents retain their existing
patch behaviour and OS-specific capability checks.

Before rolling a server back to NB1 or upstream, cancel outstanding NB2
security-only validation, approval, queued, and scheduled runs. Older servers
still see explicit selected-package lists, but do not enforce the new security
inventory freshness/reclassification check. No database migration is needed in
either direction. Never assume replacing a server container updates installed agents.

No commits, image publication, deployment, or real patching are implied by this plan.
