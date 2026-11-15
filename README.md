# mtok

**极轻量全局 token 用量检测与计费管理器。** 单个 exe，无 Electron、无守护进程、无 Node 运行时，数据全部来自本机日志，不上传。

```
  mtok  今天 11-15  138ms  12:52
  扫描 502 文件 +1；新增 39 记录 / 4.1KB / 128ms
  ──────────────────────────────────────────────────────────────
  模型                              输入    输出    缓存       ¥
  Auto(未指明)                    11.02M   1.24M 152.35M       —
  GPT-6.1-Sol                      134k   20.2k   4.89M    7.06
  GPT-6-Astra                     73.9k   11.7k   8.46M   14.26
  ──────────────────────────────────────────────────────────────
  合计                           11.23M   1.27M 165.74M   21.32
  ¥=参考价，非账单；缓存=读+写；汇率 7.20；同一底模跨厂商合并为一行
```

## 为什么不是又一个 ccusage

| | ccusage / 常见方案 | mtok |
|---|---|---|
| 每次查询 | 重新读取全部历史日志（GB 级） | 只读**上次之后新增的字节**，其余查本地聚合库 |
| 模型名 | 各厂商原名各占一行 | 归一化成官方统一名，**跨厂商合并**（`gmodel` / `glm-5-3` / `GLM_5_3` → `GLM-5.3`）|
| 新增数据源 | 只能等上游支持 | 放一个 YAML 到 `adapters.d/` 即可，**无需重新编译** |
| 运行形态 | Node 包 / 常驻 | 单个静态 exe，冷启动 ~20 ms，无常驻 |
| 计费 | 单一价格表 | 输入/输出/缓存读/缓存写分别计价 + 汇率 + 用户覆盖 + `参考价，非账单` |

## 安装

**Scoop**（推荐，清单见 `scoop/mtok.json`）

```powershell
scoop bucket add mtok https://github.com/songjiarong/scoop-mtok
scoop install mtok
```

**go install**

```powershell
go install github.com/songjiarong/mtok/cmd/mtok@latest
```

**源码构建**

```powershell
git clone https://github.com/songjiarong/mtok
cd mtok
.\scripts\build.ps1                      # 只构建本机版本
.\scripts\build.ps1 -Targets windows/amd64,linux/amd64,darwin/arm64   # 交叉编译 + hash
```

产物在 `dist/`，`bin/mtok.exe` 是可直接运行的开发版。

## 用法

```
mtok                     今天
mtok 7d | 7              近 7 天
mtok 30d                 近 30 天
mtok month               本月
mtok all                 全部历史
mtok 2026-11-05          指定某天
mtok --since 2026-11-01 --until 2026-11-15
mtok --json              机器可读（给脚本 / 状态栏用）
mtok --by-agent          按来源汇总，关闭跨厂商合并
mtok --rebuild           清空聚合库后全量重建
mtok doctor              各来源自检 + 与 magpie 对账
mtok adapters            列出 adapter、路径、prefilter、加载状态
mtok config              显示配置与数据路径（--init 生成默认配置）
mtok version
```

常用选项：`--width/-w N`（默认 64）、`--top/-n N`（默认 8）、`--usd`、`--no-magpie`、`--no-sync`（只查库不扫日志）、`--no-color`。

输出严格按终端列宽裁剪（CJK 记 2 列），管道/重定向时自动去色。

## 数据源

日志路径与字段都按本机真实样本逐条确认过，没有可读计数字段的一律标为「暂不支持」，不猜测、不估算。

| 来源 | 日志 | 字段 | 状态 |
|---|---|---|---|
| Claude Code | `~/.claude/projects/**/*.jsonl` | `message.model`、`message.usage.{input,output,cache_read_input,cache_creation_input}_tokens` | 支持（按 `message.id + requestId` 去重） |
| Codex | `~/.codex/sessions/**/rollout-*.jsonl[.zst]` | `event_msg/token_count` 的 `last_token_usage`，模型来自 `turn_context.payload.model` | 支持（`.zst` 归档整包解码一次） |
| Antigravity / Gemini | `~/.gemini/antigravity/brain/*/.system_generated/logs/transcript_full.jsonl` | `PLANNER_RESPONSE` 的 `input_tokens` / `output_tokens` / `cache_read_tokens` | 支持（模型取自日志里的 `Model Selection` 变更行） |
| Grok Build | `~/.grok/logs/*.jsonl` | `shell.turn.inference_done` 的 `prompt_tokens` / `cached_prompt_tokens` / `completion_tokens` | 支持（模型取自 `model changed`，缺省读 `~/.grok/config.toml`） |
| Kimi | `~/.kimi/sessions/**/wire.jsonl` | `StatusUpdate.token_usage.{input_other,input_cache_read,input_cache_creation,output}` | 支持（日志不含模型名，用 `config.toml` 的 `default_model`） |
| ZCode | `~/.zcode/cli/rollout/model-io-*.jsonl` | `response.usage.{inputTokens,outputTokens,cacheReadTokens,cacheWriteTokens}` + `model.modelId` | 支持 |
| pi | `~/.pi/agent/sessions/**/*.jsonl` | `message.usage.{input,output,cacheRead,cacheWrite}` | 支持 |
| dsh | `~/.dsh/sessions/**/session.v4.jsonl.zstd` | `data.usage.*`，模型 `data.message.source.model` | 支持（zstd，整包解码一次） |
| OpenCode | `~/.local/share/opencode/opencode.db` | — | **暂不支持**：用量在 SQLite 里 |
| OpenClaw | `~/.openclaw/state/*.sqlite` | — | **暂不支持**：用量在 SQLite 里 |
| Cursor | `~/.cursor/**` | — | **暂不支持**：transcript 不含 token 计数 |
| Copilot CLI | `~/.copilot/logs/*` | — | **暂不支持**：日志无 token 计数 |

`mtok doctor` 会列出本机存在的「暂不支持」来源及原因，不会静默漏掉。

## 口径（很重要）

不同厂商对「输入 token」的定义不同，mtok 统一成一套可以相加的口径：

- **输入**：不含缓存命中的部分。
  - Claude / Gemini / ZCode / dsh / pi 的日志本身就是这样（`input_tokens` 与 `cache_read` 互斥）。
  - Codex 的 `input_tokens`、Grok 的 `prompt_tokens` **包含**缓存命中，mtok 用 `input - cached` 折算，`cached` 记入缓存读。
- **输出**：厂商上报的 completion/output，含推理 token（Codex 的 `reasoning_output_tokens` 已包含在 `output_tokens` 内）。
- **缓存**：表格里的「缓存」= 缓存读 + 缓存写。
- **时间归属**：按记录时间戳换算到**本机时区**的日期；没有时间戳的记录（极少数）计入扫描当天，`mtok doctor` 会报出条数。
- **金额**：`token × 单价` 的参考价，输入/输出/缓存读/缓存写分别计价，不是账单。

## 性能

单机实测（Windows 11，NVMe，3.5 GB 日志 / 502 个文件 / 8 个来源）：

| 场景 | 耗时 |
|---|---|
| 首次建库（全量 3.5 GB，55 000 条记录） | **≈ 5.0 s** |
| 之后 `mtok` / `mtok 7d` / `mtok 30d` / `mtok all` | **0.09 ~ 0.14 s**（其中扫描 0.08~0.12 s） |
| `mtok --no-sync`（纯查聚合库） | **≈ 6 ms** + 进程启动 |
| `mtok version`（冷启动基线） | ≈ 19 ms |

为什么快：

1. `files(path, size, mtime_ns, offset)` 记住每个文件读到哪，命中「大小 + mtime 未变」直接跳过。
2. 只 `Seek(offset)` 读新增尾部，未闭合的最后一行留到下次。
3. 每行先做**子串预过滤**（如 `"token_count"`、`inference_done`），绝大多数行不进 JSON 解码器。
4. 命令只查聚合表 `(day, model, agent)`，行数在几百量级。
5. 目录遍历带前缀剪枝：`*/logs/transcript_full.jsonl` 这类模式不会钻进 `chunks/` 之类的子树。

## 架构

```
                 ┌──────────── adapters ────────────┐
 本机日志 ──glob──▶│ 内置 Go adapter（claude/codex/…）│──Record──┐
                 │ 声明式 adapter（adapters.d/*.yaml）│          │
                 └──────────────────────────────────┘          ▼
                                                       ┌──────────────┐
 指向同一文件的 offset/size/mtime ─────────────────────▶│  ingest 增量 │
                                                       └──────┬───────┘
                                                              ▼
                                        ┌───────────────────────────────┐
                                        │ 聚合库  rows(day,model,agent) │
                                        │ 单文件快照，原子重写，无 SQLite │
                                        └──────────────┬────────────────┘
                                                       ▼
                            归一化模型名 → 取价 → 渲染（CJK 列宽 / 颜色 / JSON）
```

目录：

```
cmd/mtok/                 入口（版本号注入点）
internal/adapters/        Adapter 接口、8 个内置实现、声明式引擎
internal/ingest/          glob 扫描、增量读、zstd 整包、聚合合并
internal/globx/           "**" 匹配 + 前缀剪枝
internal/store/           聚合快照（gzip+JSON，原子 rename，文件锁）
internal/model/           模型名归一化与别名表
internal/pricing/         内置价表（go:embed）+ 用户覆盖
internal/render/          表格 / 颜色 / JSON 输出
internal/magpie/          可选：额度与对账
internal/config/          路径、配置、示例文件
```

**为什么不用 SQLite**：聚合后只有几百行，快照重写 + 原子 rename 已经能在毫秒级完成，而且换来零 cgo、零依赖运行、exe 7.7 MB、交叉编译无坑。需要更复杂查询时，`--json` 已经能喂给任何工具。

## 配置

配置目录 `~/.config/mtok/`（Windows 即 `%USERPROFILE%\.config\mtok`），数据目录 `%LOCALAPPDATA%\mtok\mtok.db`（其它平台 `~/.local/share/mtok/mtok.db`）。

```powershell
mtok config --init      # 生成 config.yaml / aliases.yaml / pricing.json / adapters.d/example.yaml.txt
```

- `config.yaml`：`width`、`top`、`currency`、`rate`、`magpie`、`magpie_url`、`zstd`、`agents`（按名禁用）、`pricing_file`。
- `aliases.yaml`：`原始名或任意写法: 统一名`，覆盖内置别名表。
- `pricing.json`：与旧 `mtok.ps1` 同构（`currency.rate` / `default` / `models`），额外支持 `cache_read`、`cache_write`。
- `adapters.d/*.yaml`：声明式数据源，见 [docs/adapters.md](docs/adapters.md)。

环境变量：`MTOK_CONFIG_DIR`、`MTOK_DATA_DIR`、`MTOK_DB`（便于隔离测试）、`NO_COLOR`。

## 与 magpie 的关系（可选）

默认探测 `http://127.0.0.1:3425/v1/magpie/quotas` 显示剩余额度，`mtok doctor` 会拿 `/v1/magpie/usage?period=today` 和本地日志对账。magpie 只是可选的「额度显示 + 对账」，**不是数据依赖**：没有 magpie 也能完整使用，`--no-magpie` 完全关闭。

`magpie/auto/auto` 这类路由别名无法得知底层真实模型，会显示为 `Auto(未指明)` 且不参与计价；想计价就在 `aliases.yaml` 里把它映射到你常用的模型：

```yaml
"magpie/auto/auto": "DeepSeek-V4.1-Flash"
```

## 二次开发

- 声明式（零编译）：`~/.config/mtok/adapters.d/*.yaml`，支持 JSONL 点路径、数组下标、正则三种取数方式。
- Go 接口（走 PR）：`internal/adapters` 里实现 `Name/Agent/Globs/ParseLine`，可选实现 `Needles()` 预过滤。

详见 [docs/adapters.md](docs/adapters.md)。

## 已知限制

- Grok Build 的 `unified.jsonl` 只保留当前会话，转过的历史会丢失（mtok 无法还原），因此 grok 的长期数字会小于代理侧统计。
- Kimi / Grok 的日志不含逐请求模型名，只能用配置里的默认模型，`doctor` 里会标出。
- OpenCode / OpenClaw 的用量在 SQLite 里，当前不解析。
- 文件被截断/轮转时 offset 归零重读：`--rebuild` 才能保证不使用历史累积偏移。

## 与 ccusage 的关系

解析思路（各家日志的位置与字段形态）参考了 ccusage 这类开源工具的经验，但 mtok 不含、不调用、不依赖 ccusage，全部字段均按本机样本重新确认。

## License

MIT
