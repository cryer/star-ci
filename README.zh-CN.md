# star-ci

[English](README.md) | 简体中文

**自适应 GitHub CI：不用手写 CI 配置，工具读懂你的仓库，自动决定该跑哪些 CI。**

star-ci 扫描仓库的技术栈信号（`package.json`、`go.mod`、`pyproject.toml`、锁文件、lint/测试配置……），生成一份"项目画像"，据此推断出合理的 CI 流水线（安装依赖 → lint → 类型检查 → 测试 → 构建 → 安全扫描），然后**直接在本地/CI 容器里执行**，或者**生成一份标准的 GitHub Actions workflow**。

## 核心理念

- **零配置**：新仓库接入 CI 只需要一行 `uses:`，甚至只跑一次 `star-ci generate`。
- **只执行项目已声明的标准**：没有 eslint 配置就不会发明 lint 步骤；没有测试目录就不会发明测试。
- **置信度驱动**：每条检测都带置信度，低于阈值（0.5）的信号不会被采纳——宁可少跑，不要误跑。
- **可解释性**：每个步骤都说明"检测到了什么，因此执行什么"；`star-ci analyze` 可以查看完整证据链。
- **尊重存量**：检测到已有 `.github/workflows/` 时，`generate` 默认拒绝覆盖（除非 `--force`）。
- **安全项永远可选**：依赖漏洞扫描、密钥泄露扫描、Docker 构建等步骤标记为 optional，失败只警告不阻断。

## 工作原理

```
仓库 ──▶ Analyzer（信号扫描）──▶ Rules（信号→步骤）──▶ Plan（排序）
                                                          │
                            ┌─────────────────────────────┼────────────────────┐
                            ▼                             ▼                    ▼
                     star-ci run                   star-ci generate      star-ci analyze
                  本地/容器内直接执行              生成 workflow YAML     打印画像+证据链
```

## 安装

**从源码构建（需要 Go 1.23+）：**

```bash
git clone https://github.com/cryer/star-ci.git
cd star-ci
go build -o star-ci ./cmd/star-ci
```

或直接安装：

```bash
go install github.com/cryer/star-ci/cmd/star-ci@latest
```

**Docker：**

```bash
docker build -t star-ci .
```

## 使用流程

### 方式一：GitHub Action 运行时自适应（推荐，真零配置）

在仓库里放一个固定的 workflow（`.github/workflows/ci.yml`），这就是全部配置：

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

每次 CI 运行时，star-ci 在 job 内**现场检测**仓库并执行推断出的步骤。项目演进（换了包管理器、加了 lint）后无需改动任何配置——下次运行时自动适配。

也可以指定子目录（monorepo 场景）：

```yaml
      - uses: cryer/star-ci@v1
        with:
          path: ./packages/web
```

### 方式二：生成具体的 workflow（产物透明、可手动维护）

```bash
star-ci generate            # 写入 .github/workflows/star-ci.yml
star-ci generate --force    # 覆盖已存在的文件
star-ci generate -o ci.yml  # 自定义输出路径
```

生成的 YAML 是一份标准 GitHub Actions workflow，包含 `actions/checkout`、对应语言的 `setup-node/setup-python/setup-go`（含版本推断与依赖缓存），以及每个 CI 步骤。你可以提交后继续手动编辑。

### 方式三：本地 push 前自检

```bash
star-ci run           # 在当前目录检测并执行完整 CI 计划
star-ci run ./path    # 指定仓库目录
```

输出示例：

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

必需步骤失败会立即中断（fail-fast）；optional 步骤（安全扫描、Docker 构建）失败只警告。退出码非 0 表示必需步骤失败，可直接挂到 pre-push hook。

### 调试：查看检测结果与证据链

```bash
star-ci analyze           # 人类可读的画像 + 每条信号的置信度
star-ci analyze --json    # 机器可读的完整 ProjectProfile
```

## 检测能力矩阵

| 生态 | 入口信号 | 包管理器 | 测试 | Lint / 格式 | 类型检查 | 构建 |
|---|---|---|---|---|---|---|
| Node/TS | `package.json` | 锁文件区分 pnpm/yarn/bun/npm | `scripts.test`、vitest/jest/mocha/`node --test` | eslint / biome / prettier（需配置文件） | `tsconfig.json` + typescript → `tsc --noEmit` | `scripts.build` |
| Python | `pyproject.toml` / `requirements.txt` / `setup.py` | uv/poetry/pipenv/pip | pytest | ruff / black | mypy | — |
| Go | `go.mod` | go modules | 存在 `*_test.go` → `go test ./...` | golangci-lint（需 `.golangci.yml`） | `go vet ./...` | `go build ./...` |
| Rust | `Cargo.toml` | cargo | `cargo test` | clippy / rustfmt（需配置文件） | — | `cargo build`（有 `Cargo.lock` 时 `--locked`） |

Node monorepo 根也会被识别：`turbo.json` / `nx.json` 会让 test 与 build 步骤改走 `turbo run` / `nx run-many`，框架（`next`、`nuxt`、`remix`、`vite`）会记录在步骤的检测依据中。

**通用横切步骤（所有仓库，optional）：**

- 依赖漏洞扫描：`npm audit` / `pip-audit` / `govulncheck`（按生态匹配）
- 密钥泄露扫描：`gitleaks detect`
- 检测到 `Dockerfile` → `docker build .` 验证镜像可构建

版本推断：`.nvmrc` / `.node-version` / `engines.node`、`pyproject.toml` 的 `requires-python`、`go.mod` 的 `go` 指令、`rust-toolchain(.toml)` / `Cargo.toml` 的 `rust-version`。

## 配置（可选的 `.star-ci.yml`）

star-ci 默认零配置，但仓库根目录放一个最小 `.star-ci.yml` 即可覆盖推断出的计划：

```yaml
confidence: 0.7          # 调高/调低检测置信度阈值（默认 0.5）
disable:                 # 按 ID 禁用推断出的步骤
  - node-security
append:                  # 追加自定义步骤
  - id: docs-link-check
    name: Docs link check
    category: test       # install|lint|typecheck|test|build|security
    commands:
      - npx markdown-link-check README.md
    optional: true
```

配置生效时 `analyze` 会打印一行提示；`run` 与 `generate` 自动遵循。

## CI 报告（GitHub Job Summary）

在 GitHub Actions 中，`star-ci run` 会向 Job Summary（`$GITHUB_STEP_SUMMARY`）追加 Markdown 报告：检测到的画像、带检测依据的步骤结果表，以及可折叠的失败摘要（失败步骤输出的尾部）。本地运行时，失败步骤的错误信息同样附带该摘要。

## 项目结构

```
cmd/star-ci/        CLI 入口（analyze / run / generate）
internal/profile/   项目画像类型（ProjectProfile / Signal）——核心契约
internal/plan/      CI 步骤与计划类型（Step / Plan / Category）
internal/analyzer/  信号扫描器（node / python / go / rust / common）
internal/config/    可选的 .star-ci.yml 覆盖配置（禁用/追加步骤、置信度阈值）
internal/rules/     规则引擎：画像 → 步骤
internal/runner/    本地执行器（fail-fast + optional 警告）
internal/render/    计划 → GitHub Actions YAML
action.yml          GitHub Action（composite）
Dockerfile          容器镜像
```

## 路线图

**v1.x —— 加固（当前重点）**

- [x] `run --dry-run`：只打印推断出的计划，不执行
- [x] `.star-ci.yml` 最小覆盖配置（禁用/追加步骤、调整置信度阈值）
- [x] Reporter：GitHub Job Summary（失败摘要 + 检测依据）
- [ ] Reporter：PR 评论（失败摘要）
- [x] 更丰富的 Node 信号（turbo/nx monorepo 根、框架特定构建命令）

**v2 —— 生态与规模**

- [x] Rust（`Cargo.toml`）
- [ ] Java（Maven/Gradle）、Ruby、PHP、.NET
- [ ] monorepo workspace 检测与路径过滤（只跑受影响的包，矩阵 job）
- [ ] 测试产出覆盖率报告时的阈值检查
- [ ] 更聪明的缓存（锁文件 keyed 依赖缓存、构建产物缓存）

**v3 —— 平台化**

- [ ] GitHub App：自动注入 workflow、PR 状态报告、配置漂移后 re-generate PR
- [ ] 规则库数据驱动化（YAML 模板），社区可贡献新生态
- [ ] 自托管 runner 与容器镜像执行环境支持
