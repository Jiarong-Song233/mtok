# mtok 开发规划 / Roadmap

> 目标：把 mtok 从「自己用得爽的小工具」做成「开发者愿意主动安装、长期留在 PATH 里的 AI 用量账本」。

## 1. 定位与目标用户

一句话：**每个开发者本机的 AI 用量账本——一条命令看清今天/这段时间所有 AI 编程 agent 花了多少、花在哪个模型上、还剩多少额度。**

目标用户（按顺序）：
1. 同时用多个 AI 编程工具的个人开发者（Claude Code、Codex、Cursor、OpenCode、Kimi、Grok Build…），订阅制/按量制混用。
2. 关心成本与额度的重度用户：会被「5 小时额度用完」「月末账单超支」突然背刺的人。
3. 团队 lead / 开源维护者：需要解释 token 花在哪、给成员看账。
4. 想二次开发的数据控：自己写 adapter、接状态栏/看板。

不做什么：不做代理、不做 SaaS 后端、不做遥测、不接管流量。只读本机日志。

## 2. 要解决的痛点（按优先级）

| 优先级 | 痛点 | 现状 | 规划 |
|---|---|---|---|
| P0 | **看得见总账**：用量散落在各家日志，无法一眼汇总 | ✅ 已解决（归一化 + 跨厂商合并 + 看穿 magpie） | 维护别名表 |
| P0 | **快**：扫 GB 级日志很慢 | ✅ 已解决（增量读，<150ms） | 守性能预算 |
| P0 | **准**：缓存 token 漏算导致差 10 倍（曾 13M vs 138M） | ✅ 已解决（缓存读/写单列） | doctor 持续对账 |
| P1 | **额度**：各订阅 5h/周/月限额分散，容易撞墙 | ✅ 已解决（Codex 本地 + magpie） | 补更多本地可读来源 |
| P1 | **覆盖**：OpenCode/OpenClaw 直连、Cursor/Copilot | 🔶 经代理的用量已覆盖，直连未覆盖 | Phase 1/2：SQLite 直连（可开关） |
| P1 | **预算与告警**：想设日/月预算，超了提醒或 CI 失败 | ✅ 已解决（`--check` 退出码 3） | Phase 2：本地通知 |
| P2 | **洞察**：哪个项目/会话/模型最费钱、缓存命中率多少 | ❌ 只到「天×模型×agent」 | Phase 2：项目/会话维度 |
| P2 | **集成**：状态栏、tmux、shell prompt、Grafana | 🔶 只有 `--json` | Phase 2：导出 + 状态栏片段 |
| P2 | **团队/多机**：多台机器、团队汇总 | ❌ 无 | Phase 3：只读导入/汇总 |
| P2 | **维护**：价表/别名表会过时 | ✅ 内置 + 用户覆盖 + `pricing --fetch` | 社区 PR 流程 |
| P3 | **发布可信**：安装、升级、校验 | ✅ 首版 Release + checksums + CI + Scoop 直装 | Homebrew / 代码签名 |

**差异化护城河（要保持）**：单文件静态 exe、零运行时依赖、增量读取、看穿代理路由、跨厂商统一模型名、声明式 adapter、零遥测。这些是 ccusage / 其它计数工具没有同时具备的。

## 3. 「开发者愿意用」的质量门槛（Definition of Done）

一个功能/版本没达到这些，就不算可发布：

- **安装**：Windows `scoop install` / macOS `brew install` / Linux `curl|sh` 或 apt，一条命令；升级一条命令。
- **首跑**：5GB 日志首次建库 < 30s；之后任意查询 < 100ms；`--no-sync` < 10ms。
- **准确**：`doctor` 对支持来源逐字段对账 0.0%；口径写进文档；不猜测、不估算。
- **覆盖**：Top 15 agent 至少 12 个可读（含 OpenCode/OpenClaw 直连）；读不到的明确列出原因。
- **可靠**：进程被杀不丢已确认记录；库 schema 可迁移；日志轮转有提示且可 `--rebuild`。
- **体验**：`--help` 清楚、错误可执行、`--json` schema 稳定并带版本号、CJK 宽度/颜色/无颜色都正确。
- **跨平台**：Windows/macOS/Linux 行为一致，CI 三平台跑测试。
- **文档**：Quickstart、口径说明、adapter 编写、FAQ、CONTRIBUTING、CHANGELOG。
- **隐私**：默认零网络；唯一可选网络是读本地 magpie 和显式拉价；可审计。
- **发布**：语义化版本、CHANGELOG、校验和、（尽量）代码签名、自动更新。

## 4. 路线图

### Phase 0 — 基线（已完成，v0.2.x）
- 9 个本地 adapter（claude/codex/gemini/grok/kimi/zcode/pi/dsh + magpie journal）。
- 增量读取（offset+size+mtime）、聚合快照（gzip+JSON 原子写、文件锁）。
- 跨厂商模型归一化、输入/输出/缓存读/缓存写分别计价、总和列、`--json`。
- 看穿 magpie：route 记录按 (day,agent) 被明细替代；`doctor` 对账 0.0%。

### Phase 1 — 可发布、可被信任（v0.3，已完成主体）
1. ✅ **多来源额度看板**（v0.3.0 已完成）：从本地日志读额度，不调厂商 API、无封号风险。
   - Codex：`~/.codex/sessions/**/rollout-*.jsonl[.zst]` 的 `event_msg/token_count.rate_limits.{primary,secondary}`（含 5h/周窗口 used_percent 与 resets_at）。
   - magpie：已有 `/v1/magpie/quotas`；本地 Codex 读数优先，避免重复。
   - Claude/Gemini：本地无可靠额度字段，标注「用各自 CLI 的 /status 查看」。
   - 输出：`mtok` 底部「剩余额度」区，`mtok quota` 子命令，`--json` 含 `quotas`。
   - 缓存：首次扫描归档约 4s，之后只扫新改动文件，命中约 50ms。
   验收：本机读数与 Codex 窗口一致；`mtok quota --json` 稳定。
2. ✅ **预算门禁**（v0.3.0 已完成）：`config.yaml` 的 `budget.daily` / `budget.monthly` 按 `currency` 计价；`mtok --check` 超预算返回 3；`--json` 有 `budget` 块。
3. ✅ **JSON schema 与命令**（v0.3.0 已完成）：`--json` 带 `schema_version`，`mtok schema` 输出字段与退出码契约。
4. ⬜ **OpenCode / OpenClaw 直连**：读其 SQLite 本地库。方案：可选构建标签 `-tags sqlite`（`modernc.org/sqlite` 纯 Go），默认构建保持零依赖；或先支持「若系统有 sqlite3 可执行文件则调用」。
   验收：直连用量与工具界面一致；默认构建仍零依赖。
5. ✅ **发布工程**（v0.3.4 已完成）：公开仓库、GitHub Release（三平台产物 + checksums + CHANGELOG）、Actions 三平台测试与标签发布；Scoop 清单支持从仓库直接安装。Homebrew / 签名仍待做。
6. ✅ **文档与社区**（v0.3.4 已完成）：README Quickstart、`docs/caliber.md`、`docs/adapters.md`、CONTRIBUTING、Issue / PR 模板、SECURITY 说明。

### Phase 2 — 洞察与集成（v0.4，约 3–4 周）
- **项目/会话维度**：从 cwd/项目路径归档，输出「哪个项目最费钱」「最贵的 10 个会话」「缓存命中率」。
- **导出**：`mtok export --csv` / `--prometheus`（OpenMetrics）/ `--json` schema 文档化；附 Grafana dashboard 样例。
- **状态栏 & 实时**：`mtok statusline` 输出一行（如 `$3.2 · 12.4M tok · 5h 78%`）供 starship/tmux/nushell；`mtok watch` 轻量 TUI 刷新。
- **价表/别名更新**：内置表 + `mtok pricing --update`（本地重建）+ `mtok pricing --fetch`（LiteLLM 社区价格表在线拉取，v0.3.2 已完成）+ 社区 PR 流程。
- **预算通知**：超预算触发可配置的本地命令/桌面通知/webhook（含 Feishu，复用现有通道思路）。

### Phase 3 — 团队与规模化（v1.0）
- **多机汇总**：`mtok export --bundle` 生成脱敏快照，主控机 `mtok import` 只读合并；团队看板（本地静态 HTML 或可选服务）。
- **adapter 稳定 API + 插件目录**：第三方 adapter 分发/校验。
- **健壮性**：fuzz 解析器、10GB+ 日志性能、崩溃恢复演练、schema 迁移测试。
- **可选签名与公证**：Windows Authenticode、macOS notarization。

### Phase 4 — 生态（愿景）
- 编辑器扩展（VS Code 状态栏）、GitHub Action（PR 里贴用量/预算）、CI 门禁、Grafana 官方面板、更多 agent adapter。

## 4.1 发展方向（v0.3.4 之后）

主线是「先可靠，再洞察，最后多机」。每个阶段都保持单文件、本地优先、零遥测。

### 短期（v0.3.x）：把账本做稳

- 发布可重复：标签触发三平台构建、校验和、Release；Scoop 升级冒烟。
- 口径可复核：新来源必须有字段映射说明和回归样本，缓存读/写、别名变更都要有测试。
- 体验收尾：错误提示、`--json` schema、CJK/颜色、跨平台一致性。
- 隐私边界：默认零网络；`mtok pricing --fetch` 保持显式 opt-in。

验收目标是：一条命令装好，一条命令升级，任意支持来源都能用 `doctor` 对账。

### 中期（v0.4–v0.5）：从「看总量」到「看原因」

- 项目/会话维度：按 cwd 归档，回答「哪个项目最费」「最贵的会话是哪次」「缓存有没有省到钱」。
- 导出与集成：`--csv` / `--prometheus` / 稳定 schema，喂给 Grafana、statusline、tmux、shell prompt。
- 预算通知：超预算时执行本地命令、桌面通知或用户自己配置的 webhook；默认不外发。
- 覆盖补齐：OpenCode / OpenClaw 直连放可选 SQLite 构建；Cursor / Copilot 没有可读字段就继续明确标为不支持。

验收目标是：不看原始日志也能定位成本来源，并能把 mtok 接进现有工作流。

### 长期（v1.0）：多机账本与生态

- `export --bundle` / `import`：脱敏快照可在机器间搬运，主控机只读合并。
- adapter 稳定 API 与插件目录：第三方 adapter 可独立分发，核心只做校验和装载。
- 签名与公证：Windows Authenticode、macOS notarization，降低安装摩擦。
- 编辑器与 CI 集成：VS Code 状态栏、GitHub Action 在 PR 里贴预算摘要。

### 明确不做

- 不做代理、网关、SaaS 后端、账号体系、默认遥测。
- 不调用厂商私有接口猜额度；优先本地日志和显式配置。
- 不为覆盖而伪造字段：读不到就读不到，并在 `doctor` 里说明原因。

## 5. 关键设计决策与风险

- **SQLite 依赖 vs 零依赖**：默认构建保持零 cgo/零依赖；SQLite 直连放可选构建标签或外部 `sqlite3` 回退。破坏「单文件零依赖」是最后手段。
- **额度来源的合规**：优先读本地日志（Codex rate_limits、magpie journal）；不默认调用厂商 API，规避 ToS/封号风险。
- **模型别名/价表的可维护性**：内置 + 用户覆盖 + 可更新源；对无法确定的名字显示原名并标注「未定价」，绝不猜。
- **口径一致性**：任何新来源必须写清 input 是否含缓存、reasoning 是否计入 output，并加进 doctor 对账。
- **隐私**：默认零遥测；联网能力 opt-in 且可见。

## 6. 度量指标（North Star + 护栏）

- North Star：**周活跃开发者数 / 留存（安装后 30 天仍每周用）**。
- 护栏指标：
  - `mtok` p50 查询延迟 < 100ms；首建 < 30s。
  - `doctor` 对账差值 ≤ 0.1%（支持来源）。
  - 崩溃/丢数据报告 = 0。
  - 覆盖 agent 数 ≥ 12。
  - 安装成功率（CI 冒烟）≥ 99%。
  - 从 issue 到修复的中位时间。

## 7. 接下来 10 个可执行任务（建议顺序）

1. ✅ Codex 额度解析（读 rate_limits）+ `mtok quota` + `--json` quotas（v0.3.0）。
2. ✅ `--check` 预算门禁 + 配置项 + 退出码（v0.3.0）。
3. ✅ `--json` schema 版本号与 `mtok schema`（v0.3.0）。
4. ✅ GitHub 仓库、Actions 三平台构建 + 测试、首版 Release + checksums（v0.3.4）。
5. ✅ Scoop 清单发布（仓库直装 + autoupdate）；`go install` 冒烟；Homebrew 待做。
6. OpenCode SQLite 可选构建（`-tags sqlite`）。
7. OpenClaw SQLite 可选构建。
8. 项目/会话维度聚合（cwd → 项目）。
9. `mtok export --csv/--prometheus` + Grafana 样例。
10. `mtok statusline` 一行输出（starship/tmux 片段）。

---

维护：本文件随版本更新；每完成一项在上面勾掉并在 CHANGELOG 记录。
