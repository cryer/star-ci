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

**Cross-cutting steps (all repos, optional):**

- Dependency vulnerability audit: `npm audit` / `pip-audit` / `govulncheck` (matched to the ecosystem)
- Secret leak scan: `gitleaks detect`
- `Dockerfile` detected → `docker build .` to verify the image builds

Version inference: `.nvmrc` / `.node-version` / `engines.node`, `requires-python` in `pyproject.toml`, the `go` directive in `go.mod`.

## Project layout

```
cmd/star-ci/        CLI entrypoint (analyze / run / generate)
internal/profile/   Project profile types (ProjectProfile / Signal) — the core contract
internal/plan/      CI step & plan types (Step / Plan / Category)
internal/analyzer/  Signal scanners (node / python / go / common)
internal/rules/     Rule engine: profile → steps
internal/runner/    Local executor (fail-fast + optional warnings)
internal/render/    Plan → GitHub Actions YAML
action.yml          GitHub Action (composite)
Dockerfile          Container image
```

## Roadmap

- [ ] `.star-ci.yml` minimal override config (disable/append steps)
- [ ] Rust / Java / Ruby ecosystems
- [ ] Monorepo workspace detection + path filtering (only run CI for affected packages)
- [ ] Coverage thresholds
- [ ] GitHub App: auto-inject workflows, PR comment reports, re-generate PRs on config drift
