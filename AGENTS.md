# AGENTS.md

## 项目概述

star-ci 是一个自适应 CI 工具：扫描仓库技术栈信号生成"项目画像"，推断应执行的 CI 步骤，支持三种交付形态——本地/容器内直接执行（`run`）、生成 GitHub Actions workflow（`generate`）、结果检查（`analyze`）。

## 构建与测试

```bash
# 本机 Go 通过 scoop 安装，不在默认 PATH：
export PATH="$HOME/scoop/shims:$PATH"

go build ./...          # 编译
go vet ./...            # 静态检查
go test ./...           # 全部测试
go build -o star-ci ./cmd/star-ci   # 产出 CLI 二进制
```

端到端冒烟：对含 `package.json`/`go.mod`/`pyproject.toml` 的样例目录执行 `star-ci analyze` / `run` / `generate` 验证检测正确性。

## 硬性约定

- **仅标准库**：不得引入第三方依赖（`go.mod` 必须保持零 require）。TOML/YAML 均用行扫描/手工渲染处理。
- **契约先行**：`internal/profile`（画像）与 `internal/plan`（步骤）是全项目的数据契约，改动需同步所有使用方（analyzer/rules/runner/render/cmd）。
- **置信度纪律**：每条检测必须 `AddSignal` 记录证据与置信度；低于 `profile.MinConfidence`（0.5）的信号不得产生步骤。
- **只执行已声明的标准**：工具没有配置文件/脚本就不为其生成步骤。
- **安全类步骤一律 `Optional: true`**（失败仅警告）。
- 每个 Step 必须有引用检测依据的 `Reason`；ID 采用 `<lang>-<purpose>` 格式。
- 输出必须确定性（排序、固定顺序遍历）。

## 目录结构

| 路径 | 职责 |
|---|---|
| `cmd/star-ci/` | CLI 入口与子命令 |
| `internal/profile/` | ProjectProfile / Signal 类型 |
| `internal/plan/` | Step / Plan / Category 与排序 |
| `internal/analyzer/` | 信号扫描（node.go / python.go / golang.go / rust.go / common.go），测试用 testdata fixtures |
| `internal/config/` | 可选 `.star-ci.yml` 覆盖配置（禁用/追加步骤、置信度阈值，行扫描解析） |
| `internal/rules/` | BuildPlan：画像 → 步骤 |
| `internal/runner/` | Run（fail-fast 执行）/ Explain（干跑打印）/ GitHub Job Summary 报告（summary.go） |
| `internal/render/` | WorkflowYAML：计划 → workflow YAML（手工渲染，2 空格缩进） |
| `action.yml` / `Dockerfile` | GitHub Action（composite）与容器镜像 |

## 如何扩展

**新增生态**（如 Rust）：
1. `internal/analyzer/` 新增 `rust.go`：检测 `Cargo.toml` 等信号，写入 profile；
2. `internal/rules/rules.go` 新增对应步骤规则；
3. `internal/render/render.go` 新增 setup 步骤（如需）；
4. 每层各补测试 + analyzer 加 testdata fixture。

**新增通用步骤**：只需在 `rules.go` 追加规则函数（安全类记得 `Optional: true`）。

**用户覆盖**：`.star-ci.yml`（`internal/config`）可禁用/追加步骤、调整置信度阈值；改 plan 契约时需同步 `config.Apply`。
