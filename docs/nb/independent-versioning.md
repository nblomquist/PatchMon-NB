# Independent server and agent versions

## Release rules

- Server: nearest Git tag or explicit version, plus root `NB_BUILD`.
- Agent: the complete version in root `AGENT_VERSION`, initially `2.1.6~nb3`.
  Server tags, commit counts, and `NB_BUILD` do not change this default.
- For a server/UI-only release, increase `NB_BUILD` and leave `AGENT_VERSION`
  alone. For example, server `2.1.6~nb4` can bundle agent `2.1.6~nb3`.
- For agent source, dependency, or security-relevant toolchain/build changes,
  explicitly advance `AGENT_VERSION`. A later agent-only build could use
  `2.1.6~nb4`. When upstream agent changes are merged, advance the full agent
  version as appropriate; merging a server tag does not do this automatically.
- Never publish different agent behavior under an already distributed version.
  Version pinning is intentional release bookkeeping, not automatic change
  detection or a guarantee of byte-identical rebuilds.

The existing version ordering stays unchanged:
`2.1.6 < 2.1.6~nb1 < 2.1.6~nb2 < 2.1.7`.

## Build entrypoints

Default builds need no extra options:

```sh
mise exec -- make -C agent-source-code build
mise exec -- ./docker/build.sh --version 2.1.6 --tag local
```

To deliberately test another complete agent version without changing the pin:

```sh
mise exec -- make -C agent-source-code build AGENT_VERSION=2.1.6~nb4
AGENT_VERSION=2.1.6~nb4 mise exec -- ./docker/build.sh --tag agent-test
```

`AGENT_VERSION` accepts a complete release or prerelease version, optionally
with a positive `~nbN` suffix. No server suffix is appended to this override.
Invalid or missing pin values fail before a build.

Agent Make retains its legacy explicit `VERSION` / `NB_BUILD` overrides:
`make build VERSION=2.1.6 NB_BUILD=4` still produces `2.1.6~nb4`.
An explicit `AGENT_VERSION` takes priority over that legacy override. Scripts
building both components should pass `AGENT_VERSION` explicitly for agents,
not reuse the server's `VERSION`.

CI release-context exposes separate `version` (server) and `agent-version`
outputs. The ten-target agent matrix consumes `agent-version`; server build
and release-note consumers retain `version`. The trusted release action only
reads/validates the workspace pin; it does not execute workspace helper code.

The Docker build still compiles and bundles agents by default. This change
separates advertised versions, not the image layout or binary names. If reusing
prebuilt agents with `--skip-agents`, the operator must ensure those binaries
carry the intended agent version; the pin does not rewrite existing binaries.

## Deployment and rollback

No migration, agent protocol change, configuration change, or forced agent
rollout is required. The server already advertises the bundled binary's stamped
version as the agent's update target, rather than its own server version.
Installed `2.1.6~nb3` agents therefore remain current when a newer server bundles
that same agent version. Older installed agents can still update normally.

To roll back build tooling, restore its previous revision. Older tooling couples
agent versions to server releases again. Existing agent downgrade protections
remain unchanged; lowering the pin is not an agent downgrade mechanism.

## Verification

```sh
mise exec -- python3 -m unittest discover -s tools -p test_build_versions.py -v
mise exec -- bash -n tools/agent-version.sh docker/build.sh
```

The test suite follows red → green changes through Make, local image build
arguments, CI release-context shell execution, and the CI matrix shell loop.
It also builds the real native agent into a temporary directory and executes
only `--version`, checking both CLI output and Go build metadata. Tests of image
build arguments stop at a fake build tool; no image is built or published.
The tests run in the existing Agent Tests CI job.

Existing server/agent version comparison and binary detection tests cover the
unchanged update protocol. Full image deployment and live agent self-updates
remain release checks, not claims made by this build-tooling test suite.

### Verified 2026-10-28

- All 11 build-version tests passed, including the native binary's CLI version
  and build metadata with server `NB_BUILD=99` and agent `2.1.6~nb3`.
- Agent `make check` passed (format, vet, golangci-lint, tests).
- Server `go test -race ./internal/util -count=1` and focused race-enabled agent
  version/order/output tests passed.
- Shell syntax, changed YAML parsing, and `git diff --check` passed.
- Root Biome check passed with seven existing warnings and one informational
  diagnostic in unchanged files; no unrelated fixes were applied.
- No image was built/published and no agent was installed or upgraded. The
  Hosts page and Fetch Reports remain unchanged.
