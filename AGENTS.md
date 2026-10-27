# Repository instructions

## Fork compatibility

- Track the parent repository, `PatchMon/PatchMon`, closely. Keep fork changes
  small and focused so upstream updates remain easy to merge; avoid unrelated
  refactors, renames, dependency changes, and formatting churn.
- Keep our Docker images and agents drop-in replacements for upstream wherever
  possible. Preserve existing configuration, environment variables, ports,
  volume paths, install/update flows, API and agent protocols, and data formats.
  Prefer additive, backward-compatible changes over changing existing behaviour.
- Check compatibility with existing deployments and mixed server/agent versions
  when changing either side. Do not assume every agent updates with the server.
- If a breaking change is unavoidable, explain why and get user approval before
  implementing it; document migration and rollback requirements.

## Boundaries and entrypoints

- This is the PatchMon-NB fork. Fork plans live in `docs/nb/`; the pending-reboot
  feature is only planned in `docs/nb/host-reboot.md`, not implemented.
- Two independent Go modules: run Go commands inside `server-source-code/` or
  `agent-source-code/`, not the repository root. Both require Go 1.26.10.
  Keep the existing module paths; they are also used in linker version flags.
- `mise.toml` pins Go, golangci-lint (matching CI), and govulncheck. Run
  `mise install`; use `mise exec -- git commit ...` if the shell has not activated
  mise so the pre-commit checks can find all three tools.
- Root npm workspace contains only `frontend`. Use `npm ci` at the root and
  `npm run <script> --workspace=frontend`. Use Node 22.12+ (the locked Vite
  requires more than the root manifest's advertised Node 18 minimum).
- Server wiring: `server-source-code/cmd/server/main.go` →
  `internal/server/router.go` → `internal/handler/` → `internal/store/`.
  Stores resolve request-specific databases through `DBProvider.DB(ctx)`;
  preserve that context and registry scope checks for host actions.
- Agent server-command parsing AND dispatch live in
  `agent-source-code/cmd/patchmon-agent/commands/serve.go`. Install/remove scripts
  are embedded from `server-source-code/internal/agents/`, not stored alongside
  the generated agent binaries.

## Build and local development

- `npm run build` at the root builds only the frontend, not the server or agents.
- Before building/testing the full server module, prepare its embedded UI (from
  the repository root):
  ```sh
  npm run build --workspace=frontend
  mkdir -p server-source-code/cmd/server/static/frontend
  cp -r frontend/dist server-source-code/cmd/server/static/frontend/
  ```
  `cmd/server/embed.go` requires that directory. When refreshing an existing
  embed, remove only its generated `dist/` first to avoid accumulating old hashed
  assets. Neither `make build` nor Air builds/copies the frontend for you.
- `npm run dev:server` needs Air installed. Server config loads `.env` from its
  working directory (`server-source-code/` via npm), or `ENV_FILE` if set.
  Vite listens on 3000 and proxies API/WebSocket traffic to 3001 by default;
  set backend `PORT=3001` (server default is 3000), or override Vite's
  `SERVER_HOST`/`SERVER_PORT`. Running the server needs PostgreSQL and Redis;
  startup applies migrations automatically.

## Verification

- In either Go module: `go test ./internal/<package> -run '^TestName$' -count=1`
  for a focused test; `make check` runs format check → vet → golangci-lint → tests.
  CI uses golangci-lint v2.13.1 and race-enabled Go tests.
- Server database tests use `PATCHMON_TEST_DATABASE_URL`, not `DATABASE_URL`.
  Use a disposable PostgreSQL 17 instance with a role allowed to create/drop
  databases: `internal/pgtest` creates and migrates one per test binary. Tests
  using it skip without the URL locally and fail without it when `CI` is set;
  a passing local suite may therefore omit database coverage.
- Frontend: `npm run test:run --workspace=frontend`; focused example:
  `npm run test:run --workspace=frontend -- src/__tests__/components/HostIdentity.test.jsx`.
  Tests use jsdom and `frontend/src/__tests__/setup.js`; `npm test` is not a root
  script and the frontend `test` script watches rather than exiting.
- Root `npm run lint` uses Biome, not ESLint. `npm ci` installs Lefthook hooks:
  commits run both Go `make check` targets, frontend tests, Biome, production
  dependency audit, and `govulncheck` for both modules. The secrets hook is
  currently disabled despite the header comment; see `lefthook.yml`.
  Hooks run sequentially: parallel Go linters conflict on their shared lock,
  and concurrent suites can starve frontend interaction tests.
- Go 1.26 test temporary directories follow the host umask. Tests that require
  a trusted private cache must explicitly set fixture permissions; do not relax
  production cache ownership/writability checks to make those tests pass.

## Database and release artifacts

- Edit SQL in `server-source-code/internal/sqlc/queries/` and
  `internal/sqlc/schema/schema.sql`, then run `make sqlc-generate` in the server
  module (sqlc v1.30.0). `internal/db/` is generated. Runtime migrations are
  separate embedded SQL files in `internal/migrate/migrations/`; schema edits
  alone do not migrate deployments.
- Use `./docker/build.sh --version X.Y.Z --tag local` from the root for a local
  image. A plain Docker build on a clean checkout fails: the production Dockerfile
  copies ignored `server-prebuilt/` and `agents-prebuilt/`, it does not compile
  them. The script builds both modules and the embedded UI first.
- Build versions come from the nearest Git tag or explicit `VERSION`, not
  `package.json`. Local Docker builds require bare `MAJOR.MINOR.PATCH`; CI has
  its own prerelease logic in `.github/actions/release-context/`.
- The image serves bundled agents from `/app/agents` (`AGENTS_DIR`); replacing
  the server container does not itself update installed agents. Rebuild the
  bundled agents when changing their command protocol.
- Fork image name is `patchmon-nb`: local builds default to `patchmon-nb:local`
  (`IMAGE_NAME` overrides it); CI publishes to `ghcr.io/<owner>/patchmon-nb`.
  GHCR package visibility and access are separate from repository visibility.
- For upstream submissions, follow the AI disclosure requirements in
  `CONTRIBUTING.md` and `AI-DECLARATION.md`.
