# star-ci

English | [简体中文](README.zh-CN.md)

**Adaptive CI for GitHub: no hand-written CI config — the tool reads your repository and decides which CI to run.**

star-ci scans a repository's tech-stack signals (`package.json`, `go.mod`, `pyproject.toml`, lockfiles, lint/test configs, …), builds a *project profile*, and infers a sensible CI pipeline (install deps → lint → typecheck → test → build → security scans). It can then **execute the plan directly** (locally or inside a CI container) or **render a standard GitHub Actions workflow**.

## Principles

- **Zero config**: onboarding a repo to CI takes a single `uses:` line, or one `star-ci generate` run.
- **Only runs standards the project already declares**: no eslint config → no lint step invented; no tests → no test step invented.
- **Confidence-driven**: every detection carries a confidence score; signals below the threshold (0.5) are ignored — better to skip a step than to run a wrong one.
- **Explainable**: every step states *what was detected and why it runs*; `star-ci analyze` shows the full evidence trail.
- **Respects existing CI**: when `.github/workflows/` already exists, `generate` refuses to overwrite (unless `--force`).
- **Security steps are always optional**: dependency audits, secret scanning, and Docker builds warn instead of failing the run.

## How it works

```
repo ──▶ Analyzer (signal scan) ──▶ Rules (signals → steps) ──▶ Plan (ordering)
                                                                  │
                            ┌─────────────────────────────────────┼────────────────────┐
                            ▼                                     ▼                    ▼
                     star-ci run                           star-ci generate      star-ci analyze
                  execute locally / in a container      render workflow YAML   print profile + evidence
```

## Installation

**Build from source (requires Go 1.23+):**

```bash
git clone https://github.com/cryer/star-ci.git
cd star-ci
go build -o star-ci ./cmd/star-ci
```

Or install directly:

```bash
go install github.com/cryer/star-ci/cmd/star-ci@latest
```

**Docker:**

```bash
docker build -t star-ci .
```

## Usage

### Option 1: GitHub Action with runtime adaptation (recommended, truly zero-config)

Put one fixed workflow in your repo (`.github/workflows/ci.yml`) — that's the entire configuration:

```yaml
name: ci
on: [push, pull_request]
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: cryer/star-ci@v1
```

On every run, star-ci **analyzes the repo live inside the job** and executes the inferred steps. As the project evolves (new package manager, added linting), nothing needs updating — the next run adapts automatically.

You can also point it at a subdirectory (monorepos):

```yaml
      - uses: cryer/star-ci@v1
        with:
          path: ./packages/web
```

### Option 2: Generate a concrete workflow (transparent, hand-editable output)

```bash
star-ci generate            # writes .github/workflows/star-ci.yml
star-ci generate --force    # overwrite an existing file
star-ci generate -o ci.yml  # custom output path
```

The generated YAML is a standard GitHub Actions workflow: `actions/checkout`, the matching `setup-node`/`setup-python`/`setup-go` (with version inference and dependency caching), and one step per CI task. Commit it and keep editing by hand if you like.

Dependency caching is lockfile-keyed: ecosystems with built-in setup caching (node, python, go, java, ruby) get the right `cache` / `cache-dependency-path` settings, and Rust/PHP/.NET get explicit `actions/cache` steps (including the Cargo `target` build-artifact cache). In monorepo matrix jobs, cache paths are prefixed per workspace.

### Option 3: Local pre-push self-check

```bash
star-ci run           # detect + execute the full CI plan in the current directory
star-ci run ./path    # ...or in a specific repo
```

Example output:

```
star-ci: detected node, 5 steps
==> Install dependencies (npm) (install)
    reason: package.json + package-lock.json detected
    $ npm ci
...
==> Scan for secrets (gitleaks) (security)
    warning: optional step security-secrets failed: exit status 127
star-ci: 4 steps passed, 1 optional warnings
```

A failing required step aborts the run (fail-fast); optional steps (security scans, Docker build) only warn. A non-zero exit code means a required step failed, so it drops straight into a pre-push hook.

### Debugging: inspect detections and the evidence trail

```bash
star-ci analyze           # human-readable profile + per-signal confidence
star-ci analyze --json    # machine-readable full ProjectProfile
```

## Detection matrix

| Ecosystem | Entry signals | Package manager | Tests | Lint / format | Typecheck | Build |
|---|---|---|---|---|---|---|
| Node/TS | `package.json` | lockfile: pnpm/yarn/bun/npm | `scripts.test`, vitest/jest/mocha/`node --test` | eslint / biome / prettier (config file required) | `tsconfig.json` + typescript → `tsc --noEmit` | `scripts.build` |
| Python | `pyproject.toml` / `requirements.txt` / `setup.py` | uv/poetry/pipenv/pip | pytest | ruff / black | mypy | — |
| Go | `go.mod` | go modules | any `*_test.go` → `go test ./...` | golangci-lint (requires `.golangci.yml`) | `go vet ./...` | `go build ./...` |
| Rust | `Cargo.toml` | cargo | `cargo test` | clippy / rustfmt (config file required) | — | `cargo build` (`--locked` with `Cargo.lock`) |
| Java | `pom.xml` / `build.gradle(.kts)` | maven / gradle (wrapper preferred) | `mvn -B test` / `gradle test` | — | — | `mvn -B package -DskipTests` / `gradle build -x test` |
| Ruby | `Gemfile` | bundler | rspec (in Gemfile) / `rake` | rubocop (requires `.rubocop.yml`) | — | — |
| PHP | `composer.json` | composer | phpunit (`phpunit.xml(.dist)` or require-dev) | — | — | — |
| .NET | `*.sln` / `*.csproj` | nuget | `dotnet test` (test project required) | — | — | `dotnet build` |
| C/C++ | `CMakeLists.txt`, or `Makefile` + C/C++ sources | — | `ctest` (requires `enable_testing()`) / `make test` | — | — | `cmake --build` / `make` |

Node monorepo roots are recognized too: `turbo.json` / `nx.json` route test & build through `turbo run` / `nx run-many`, and frameworks (`next`, `nuxt`, `remix`, `vite`) are recorded in the step rationale.

### Monorepo workspaces

Workspace declarations are detected at the repo root — `package.json` `workspaces`, `pnpm-workspace.yaml` `packages`, Cargo.toml `[workspace] members`, `go.work` `use` — and glob patterns are expanded to real directories (entries without the matching manifest are skipped). `analyze` lists the detected workspaces.

- `star-ci run --changed-since <git-ref>` computes changed files via `git diff --name-only <ref>...HEAD` and only analyzes + runs the affected workspaces (a changed file outside every workspace — e.g. a root lockfile or manifest — affects all of them). Zero changed files exits successfully without running anything; a failing workspace aborts the run (fail-fast). Without detected workspaces the flag is an error. Requires git.
- `star-ci generate` renders one job with a `strategy.matrix.workspace` when every workspace yields the same steps (each instance runs them with `working-directory: ${{ matrix.workspace }}`); when the per-workspace plans differ, it falls back to one job per workspace named after the workspace path.

**Cross-cutting steps (all repos, optional):**

- Dependency vulnerability audit: `npm audit` / `pip-audit` / `govulncheck` (matched to the ecosystem)
- Secret leak scan: `gitleaks detect`
- `Dockerfile` detected → `docker build .` to verify the image builds

Version inference: `.nvmrc` / `.node-version` / `engines.node`, `requires-python` in `pyproject.toml`, the `go` directive in `go.mod`, `rust-toolchain(.toml)` / `rust-version` in `Cargo.toml`.

## Configuration (optional `.star-ci.yml`)

star-ci is zero-config by default, but a minimal `.star-ci.yml` at the repo root can override the inferred plan:

```yaml
confidence: 0.7          # raise/lower the detection threshold (default 0.5)
coverage: 80             # run fails when measured line coverage is below this (0-100)
disable:                 # drop inferred steps by ID
  - node-security
append:                  # add your own steps
  - id: docs-link-check
    name: Docs link check
    category: test       # install|lint|typecheck|test|build|security
    commands:
      - npx markdown-link-check README.md
    optional: true
```

`analyze` prints a line when a config is in effect; `run` and `generate` honor it automatically.

The `coverage` threshold only applies to `run`: after all steps pass, star-ci looks for a known coverage report — `coverage/coverage-summary.json` (vitest/jest), `coverage.xml` (pytest-cov/coverage.py), or `cover.out` / `coverage.out` (`go test -coverprofile`) — and fails the run when the measured line coverage is below the declared percentage. With several reports the lowest percentage wins; with no report the check is skipped (tests simply did not emit coverage).

## CI reports (GitHub Job Summary)

Inside GitHub Actions, `star-ci run` appends a Markdown report to the [Job Summary](https://github.blog/news-insights/product-news/supercharging-github-actions-with-job-summaries/) (`$GITHUB_STEP_SUMMARY`): the detected profile, a per-step result table with each step's detection rationale, and collapsible failure digests (the tail of the failed step's output). Locally, a failing step's error message carries the same digest.

## Project layout

```
cmd/star-ci/        CLI entrypoint (analyze / run / generate)
internal/profile/   Project profile types (ProjectProfile / Signal) — the core contract
internal/plan/      CI step & plan types (Step / Plan / Category)
internal/analyzer/  Signal scanners (node / python / go / rust / common) + workspace detection (workspaces.go)
internal/config/    Optional .star-ci.yml overrides (disable/append steps, confidence, coverage)
internal/rules/     Rule engine: profile → steps
internal/runner/    Local executor (fail-fast + optional warnings)
internal/render/    Plan → GitHub Actions YAML
action.yml          GitHub Action (composite)
Dockerfile          Container image
```

