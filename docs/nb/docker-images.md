# Fork Docker images

## Names and compatibility

- Local image: `patchmon-nb:local` (or `patchmon-nb:smoke` for testing).
- GitHub Container Registry: `ghcr.io/nblomquist/patchmon-nb`.
- Only the image name changes. Keep upstream service names, ports, environment
  variables, volume paths, and bundled agent executable names compatible.
- `docker/docker-compose.yml` references the fork image. Its `latest` tag needs
  a published release before it can be pulled; merely editing the workflow does
  not create that image.

## Build locally

```sh
mise exec -- ./docker/build.sh --tag smoke
```

The script stamps the nearest Git tag plus `~nbN` into the server and agents.
`N` comes from the root `NB_BUILD` file (currently `2`). If this fork
has no tags, fetch the parent's tags locally first:

```sh
git fetch https://github.com/PatchMon/PatchMon.git --tags
```

Alternatively, supply `--version X.Y.Z`; the fork marker is appended automatically.
Increase `NB_BUILD` for the next distributed fork build (or override `NB_BUILD`
for a deliberate local test). Do not invent a newer upstream version just to
distinguish the fork. Versions remain ordered by upstream version first, then
the numeric NB build number. The same upstream release without an NB marker sorts
below its NB builds, so same-release upstream builds do not replace fork agents.

Docker tags cannot contain `~`. For example, build `patchmon-nb:nb1` with an
application/agent version of `2.1.6~nb1`:

```sh
mise exec -- ./docker/build.sh --tag nb1
```

The build compiles Linux, FreeBSD, and Windows agents, embeds the frontend in
Linux server binaries, and packages the agents into the server image. It needs
Go, Node/npm, Make, Docker, and network access to dependencies and base images.
`IMAGE_NAME` can override the local name without changing the Dockerfile.

The upstream production base is `dhi.io/alpine-base:3.23` (Docker Hardened
Images). The build host needs registry access via `docker login dhi.io` before
building. This is separate from GHCR authentication; hosts pulling our finished
GHCR image do not need a separate DHI login.

## Deployment test

Use a separate Compose project with fresh PostgreSQL and Redis volumes, generated
test secrets, and a loopback-only port. Do not point the test at production data:
server startup applies migrations automatically.

Verify that:

- All services become healthy, including the guacd sidecar.
- `/health` reports database and Redis connectivity.
- `/` serves the embedded frontend and its JavaScript/CSS assets.
- `/app/agents` contains binaries for all ten supported OS/architecture targets.
- The native agent binary's version matches the server build version; execute
  only its version command, not an agent install or service startup.

Record results here after the test. A local smoke test does not prove private
registry access from Fenrir, agent self-updates, or all application features.

### Build attempt: 2026-10-27

- Source commit: `d6e91df2`, with the fork image-name changes in the working tree.
- `mise exec -- ./docker/build.sh --tag smoke` used upstream version `2.1.6`.
- Successfully compiled all ten agent targets, the production frontend, and
  Linux amd64/arm64 server binaries.
- Docker packaging failed fetching `dhi.io/alpine-base:3.23`: anonymous access
  returned HTTP 401. No `patchmon-nb:smoke` image was produced or deployed.
- Compose configuration validation and `bash -n docker/build.sh` passed.
- Dependency installation reported seven npm vulnerabilities (six moderate,
  one high); these were not investigated or changed in this image-name task.
- Next: authenticate to DHI, rerun the build, then perform the isolated runtime
  checks above. Private GHCR publishing and a Fenrir pull remain untested.

### Successful retry: 2026-10-27

- After DHI login, the same build command produced `patchmon-nb:smoke` reporting
  version `2.1.6`. Local image ID:
  `sha256:ce3f54ee30fc0be93536c65cbe6df7e3e4aff5496f164f82dad04c63f9454816`.
- Started the production Compose layout as project `patchmon-nb-smoke`, with
  separate PostgreSQL/Redis volumes and generated test secrets. Only the server
  port is published, at `127.0.0.1:33080`; restart policies are disabled for this
  temporary test.
- `docker compose up -d --wait --wait-timeout 180` passed: server, database,
  Redis, and guacd all became healthy. Startup applied embedded migrations to
  schema version 59 on the fresh database.
- `/health?format=json` returned
  `{"status":"ok","database":"ok","redis":"ok"}`.
- `/` served the embedded React HTML; its referenced logo, JavaScript, and CSS
  returned non-empty assets with appropriate non-HTML content types.
- `/app/agents` contained all ten agent binaries. Running
  `/app/agents/patchmon-agent-linux-amd64 --version` inside the container returned
  `patchmon-agent version 2.1.6`. No agent was installed or enrolled.
- This exercised the native Linux amd64 container only. Linux arm64 server and
  other agent targets compiled but were not executed. Application suites,
  existing-database upgrades, private GHCR publishing, and Fenrir pulls remain
  untested.

The temporary test files (including private generated `.env`) are outside the
repository at `/tmp/opencode/patchmon-nb-smoke-b1txl1m_/`. To stop this test only:

```sh
docker compose -p patchmon-nb-smoke \
  -f /tmp/opencode/patchmon-nb-smoke-b1txl1m_/compose.yml \
  --env-file /tmp/opencode/patchmon-nb-smoke-b1txl1m_/.env down
```

This retains its test volumes. Do not reuse these temporary secrets or data for
production.

## Private GHCR publishing and Dockhand

- `.github/workflows/docker.yml` publishes under the owner's `patchmon-nb`
  package: release tags produce version tags and `latest`; `main` produces `edge`.
- CI version resolution requires tags in the fork too. Fetching tags locally does
  not make them available to GitHub Actions. Review which tag/branch to publish
  before pushing; tag pushes can trigger release workflows.
- A successful local DHI login does not authenticate GitHub Actions. Before CI
  publishing, add a secure DHI registry login to the image-build job; the current
  workflow only logs in to GHCR and optionally Harbor. Do not expose registry
  credentials to untrusted pull-request code.
- Keep the GHCR package private and verify its visibility in GitHub package
  settings after publishing. Source-repository visibility is separate.
- In Dockhand, configure a registry at `https://ghcr.io`, username `nblomquist`,
  and a classic GitHub token with `read:packages` and access to the package.
  Keep Dockhand's database on persistent appdata storage; do not put the token
  in Compose files or this repository.
- For an existing deployment, back up its database first, change only the server
  image reference to an explicitly tested tag/digest, and preserve its settings
  and volumes. Do not use the upstream setup downloader to overwrite the fork's
  Compose file.
