# 贡献指南

欢迎补 adapter、模型别名、价格表、文档和测试。mtok 的定位是本地优先、离线可用、零遥测的小工具，所有贡献都应保持这个边界。

## 开发环境

- Go 1.22 或更新版本。
- PowerShell 7 用于 `scripts/build.ps1`；直接 `go build ./cmd/mtok` 不需要 PowerShell。
- 可选：Scoop（Windows）、magpie（用于本地对账）。

常用命令：

```powershell
go test ./...
go vet ./...
gofmt -l .
go build ./cmd/mtok
```

交叉编译：

```powershell
pwsh scripts/build.ps1 -Version 0.3.4 -Targets windows/amd64,linux/amd64,darwin/arm64
```

产物在 `dist/`，`checksums.txt` 包含 SHA256。

## 可以贡献什么

### 新 adapter

1. 先确认本机真实日志里有可读的 token 字段，不要按厂商文档猜测。
2. 在 `internal/adapters/` 实现内置 adapter，或写 `~/.config/mtok/adapters.d/*.yaml` 声明式 adapter。
3. 字段口径写进 `docs/caliber.md`。
4. 加解析测试，覆盖正常行、缺字段、缓存字段和异常行。
5. 说明时间归属、模型名来源、是否含缓存，以及如何对账。

### 模型别名

别名在 `internal/model/aliases.go`。同一底模的不同写法应合并到一个 canonical 名。改别名不会追溯修正已入库记录，需要重建本地聚合库后再验证。

注意：`internal/model/normalize_test.go` 有硬编码的 canonical 期望，改别名时同步更新。

### 价格表

内置价表在 `internal/pricing/builtin.json`，单位 USD / 百万 token。`mtok pricing --fetch` 会从 LiteLLM 社区表拉价并写本地 `pricing.json`。

提交价格变更时请附上来源 URL 和抓取日期，写清输入、输出、缓存读、缓存写四个单价。无法确定时保留原名并显示「未定价」，不要猜。

### 文档

README 负责 Quickstart；`docs/caliber.md` 负责口径；`docs/adapters.md` 负责数据源写法；`docs/ROADMAP.md` 负责方向。修行为时优先改文档，避免只有代码里知道规则。

## 提交与 PR

- 一个 PR 做一件事，标题写清楚改了哪个来源或哪个行为。
- 附上 `go test ./...` 结果；涉及输出变化时贴 `mtok` 前后对比。
- 不要提交真实日志、配置、token、额度截图或本机路径中的个人信息。
- 涉及新网络能力必须先讨论：默认构建保持零网络，所有联网行为都要显式触发并写进文档。

## 发布

维护者按语义化版本打 tag：`v0.3.4`。`release.yml` 会在 tag push 后构建三平台产物、生成 `checksums.txt` 并创建 Release。

Release 之后需要把 Windows 产物的 hash 写回 `scoop/mtok.json`，并推送到 `main`，让 `scoop update mtok` 能升级到新版本。

## 行为边界

- 不做代理、网关、SaaS 后端、账号体系。
- 不默认调用厂商 API，不读取与统计无关的私有接口。
- 不上传用量数据；唯一在线功能是用户显式运行的 `mtok pricing --fetch`。
