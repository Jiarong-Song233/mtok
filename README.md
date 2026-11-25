# mtok

**极轻量全局 token 用量检测与计费管理器。** 单个 exe，无 Electron、无守护进程、无 Node 运行时，数据全部来自本机日志，不上传。

```
  mtok  今天 11-18  109ms  13:22
  扫描 503 文件 +2；新增 4 记录 / 3.5KB / 101ms / 查询 0ms
  ──────────────────────────────────────────────────────────────
  模型                 输入   输出    缓存    总和       $
  DeepSeek-V4.1-Flash  9.3M  1.17M 184.31M 195.0M   91.76
  Hy4                 3.95M   403k  32.03M  36.4M   25.82
  GPT-6-Astra         73.9k  11.7k   8.47M  8.55M   14.26
  GPT-6.1-Sol          134k  20.2k   4.90M  5.04M    7.06
  Codex-Auto-Review  180.5k  1.61k  828.2k  1.01M       —
  GLM-5.3-Flash      292.8k  3.24k  289.9k 585.9k    0.26
  …另有 5 行  250 tok
  ──────────────────────────────────────────────────────────────
  合计                13.89M  1.6M 247.50M 262.98M     138
  $ 为估算值，非账单/非实际结算
  magpie 明细 11 条，覆盖 1 条旧记录；直连日志照常计入
  未定价: Codex-Auto-Review
  ──────────────────────────────────────────────────────────────
  剩余额度
  codex(本地) 5h84% 7d32% cursor 88%    grok 100%
  qoder-cn 41%×2     raccoon 23%        trae-cn 2%
  workbuddy 39%×2
```

## 为什么不是又一个 ccusage

| | ccusage / 常见方案 | mtok |
|---|---|---|
| 每次查询 | 重新读取全部历史日志（GB 级） | 只读**上次之后新增的字节**，其余查本地聚合库 |
| 模型名 | 各厂商原名各占一行 | 归一化成官方统一名，**跨厂商合并**（`gmodel` / `glm-5-3` / `GLM_5_3` → `GLM-5.3`）|
| 代理路由 | 只看到 `auto/auto`，全部糊成一行 | **看穿 magpie**：用代理的调用明细还原 `served` 真实模型（`DeepSeek-V4.1-Flash` / `Hy4` / `GLM-5.3-Flash`…），并保证不与本地日志重复计 |
| 新增数据源 | 只能等上游支持 | 放一个 YAML 到 `adapters.d/` 即可，**无需重新编译** |
| 运行形态 | Node 包 / 常驻 | 单个静态 exe，冷启动 ~20 ms，无常驻 |
| 计费 | 单一价格表 | 输入/输出/缓存读/缓存写分别计价 + 美元结算 + 用户覆盖 + `参考价，非账单` |

## 安装

**Scoop**（推荐，清单随仓库发布）

```powershell
scoop install https://raw.githubusercontent.com/Jiarong-Song233/mtok/main/scoop/mtok.json
```

升级用 `scoop update mtok`。

**go install**

```powershell
go install github.com/Jiarong-Song233/mtok/cmd/mtok@latest
```

**源码构建**

```powershell
git clone https://github.com/Jiarong-Song233/mtok
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
mtok --check             预算门禁：超预算退出码 3
mtok --by-agent          按来源汇总，关闭跨厂商合并
mtok --rebuild           清空聚合库后全量重建
mtok quota               只看剩余额度（Codex 本地 + magpie）
mtok doctor              各来源自检 + 与 magpie 对账
mtok adapters            列出 adapter、路径、prefilter、加载状态
mtok config              显示配置与数据路径（--init 生成默认配置）
mtok pricing             显示价格表来源（--update 本地重建；--fetch 从 LiteLLM 在线拉取）
mtok schema              输出 --json 的稳定 schema 描述
mtok version
```

常用选项：`--width/-w N`（默认 72）、`--top/-n N`（默认 8）、`--usd`（默认已用美元）、`--no-magpie`（不读代理明细与代理额度，本地额度仍读）、`--no-sync`（只查库不扫日志）、`--no-color`。

金额默认以美元结算，价格单位是 USD/百万 token。`缓存读` 和 `缓存写` 分开计价。想改回人民币，在 `~/.config/mtok/config.yaml` 写 `currency: CNY` 和 `rate`。改价直接编辑 `~/.config/mtok/pricing.json`；`mtok pricing --update` 从内置表重建，`mtok pricing --fetch` 从 LiteLLM 社区价格表在线拉取并更新本地文件。`mtok pricing` 显示当前价格来源与路径。

额度来自三个本地源，全程离线：Codex `sessions/**/rollout-*.jsonl[.zst]` 的 `rate_limits`；T3 网关缓存 `~/.t3/caches/*.json`（Gemini、Claude Code、Grok 等的 5 小时/周额度）；以及本地 magpie 代理额度。首次读取归档约几秒，之后只扫改动文件。`--json` 带 `schema_version`，契约用 `mtok schema` 查看。预算写在 `~/.config/mtok/config.yaml` 的 `budget.daily` / `budget.monthly`，单位是 `currency` 指定的币种。T3 未接入的 agent 才需要去各自 CLI 的 `/status` 看额度。

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
| **magpie 代理** | `~/.config/magpie/usage.jsonl` | 每次调用的 `served`（真实模型）、`in/out/cache_read/cache_write`、`agent` | 支持（**看穿路由**，见下节） |
| OpenCode | `~/.local/share/opencode/opencode.db` | — | 经 magpie 的部分已由明细覆盖；直连部分**暂不支持**（SQLite） |
| OpenClaw | `~/.openclaw/state/*.sqlite` | — | 经 magpie 的部分已由明细覆盖；直连部分**暂不支持**（SQLite） |
| Cursor | `~/.cursor/**` | — | **暂不支持**：transcript 不含 token 计数 |
| Copilot CLI | `~/.copilot/logs/*` | — | **暂不支持**：日志无 token 计数 |

`mtok doctor` 会列出本机存在的「暂不支持」来源及原因，不会静默漏掉。

## 看穿 magpie（核心）

Grok Build、OpenCode、dsh、pi 这些 agent 在本机是**走 magpie 网关**的，日志里只写 `auto/auto` 或 `group/auto-auto`。只看本地日志，这些用量只能糊成一行 `Auto(未指明)`——而它们往往占当天 90% 以上。

mtok 的做法：

1. **读代理自己的调用明细** `~/.config/magpie/usage.jsonl`（每次调用一行，含 `served`、`in/out/cache_read/cache_write`、`agent`、时间戳）。`served` 是上游真正跑的模型，所以 `auto`、`qfmodel`、`hy4free` 这类路由别名都能落到 `DeepSeek-V4.1-Flash`、`Qwen3.8-Flash`、`Hy4`。
2. **本地日志只用来判定「是不是 route」**：模型名是 `auto/auto`、`auto/*`、`group/*`、`magpie/*`、`auto-*` 的记录被标记为 route，不带模型身份。
3. **去重**：某天的某个 agent 只要有明细，它当天的 route 记录就整段被明细替代（按 `(day, agent)` 覆盖，不跨 agent、不跨天）。直连日志（Claude→Anthropic、Codex→OpenAI、Gemini、Kimi、ZCode）照常计入。
4. **兜底**：明细没覆盖到的日子（journal 只保留最近一段），route 记录原样保留，仍显示为 `Auto(未指明)`，不会丢数据。
5. `mtok --no-magpie` = 纯本地模式：不读明细、不查额度，行为与之前完全一致。

```
  合计                 13.89M  1.6M 247.2M    295k 262.98M     138
  magpie 明细 11 条，覆盖 1 条旧记录；直连日志照常计入
```

**为什么用 journal 而不是只调 API**：`/v1/magpie/usage` 只支持 `today` / `7d` / `30d` / `all` 四种固定周期（传 `since/until` 会被静默忽略），无法对齐 `mtok 7d` 的按日窗口，也无法支持 `--since/--until`。journal 正是该接口的数据源（API 响应里的 `path` 字段就是它），逐行带时间戳，于是任意区间、任意按日切片都能精确对齐。API 仍然被使用——`mtok doctor` 用它对账。

**逐字段对账**（`mtok doctor`，与 `/v1/magpie/usage` 的 `models[]` 求和）：

```
         input        output       cache_read   calls
  today  13501594     1563424      216219072    1248          mtok journal
   api   13501594     1563424      216219072    1281          models[] 求和
         差值  +0.0% / +0.0% / +0.0%
  7d     118642236    4743475      639196557    4817
   api   118642236    4743475      639196557    4954
         差值  +0.0% / +0.0% / +0.0%
  30d    289303404    10862386     1412553666   13950
   api   289303404    10862386     1412553666   14261
         差值  +0.0% / +0.0% / +0.0%
```

- **三列 token 逐字段完全一致**；模型行名可能不同：mtok 按 `served`（真实底层模型）分组，magpie 的 `models[]` 按 `model`（可能是 `hy4free`/`qfmodel` 这类路由码）分组，总量相同。
- `calls` 略少：mtok 不计 0-token 记录（错误/取消），magpie 仍计次。
- 当天仍在写入时会有个位数漂移（两次采样时间不同）。

## 口径（很重要）

不同厂商对「输入 token」的定义不同，mtok 统一成一套可以相加的口径：

- **输入**：不含缓存命中的部分。
  - Claude / Gemini / ZCode / dsh / pi 的日志本身就是这样（`input_tokens` 与 `cache_read` 互斥）。
  - Codex 的 `input_tokens`、Grok 的 `prompt_tokens` **包含**缓存命中，mtok 用 `input - cached` 折算，`cached` 记入缓存读。
- **输出**：厂商上报的 completion/output，含推理 token（Codex 的 `reasoning_output_tokens` 已包含在 `output_tokens` 内）。
- **缓存**：表格里的「缓存」列 = 缓存读 + 缓存写 合并显示；计价时仍按缓存读、缓存写各自的单价分开算。
- **总和**：表格里的「总和」= 输入 + 输出 + 缓存（读 + 写）。
- **时间归属**：按记录时间戳换算到**本机时区**的日期；没有时间戳的记录（极少数）计入扫描当天，`mtok doctor` 会报出条数。
- **金额**：`token × 单价` 的参考价，输入/输出/缓存读/缓存写分别计价，不是账单。

## 性能

单机实测（Windows 11，NVMe，3.5 GB 日志 + 10 MB 代理明细 / 503 个文件 / 9 个来源，`go1.27.4 windows/amd64`）：

| 场景 | 耗时 |
|---|---|
| 首次建库（全量 3.5 GB，70 250 条记录） | **4.9 s** |
| 之后 `mtok` / `mtok 7d` / `mtok 30d` / `mtok all`（含增量扫描） | **中位数 101–109 ms**（min 89 / max 197，n=12） |
| `mtok --no-sync`（纯查聚合库） | **< 1 ms**，进程总耗时 ≈ 20 ms |
| `mtok version`（冷启动基线） | ≈ 17 ms |

≈ 46 倍。剩余耗时几乎全是目录遍历（`~` 下 4000+ 个条目的 stat/枚举），解析与聚合可以忽略；`--no-sync` 直接把扫描省掉，查询本身在亚毫秒级。代理明细只是一行一次追加的 JSONL，同样走增量偏移，几乎不增加成本。

为什么快：

1. `files(path, size, mtime_ns, offset)` 记住每个文件读到哪，命中「大小 + mtime 未变」直接跳过。
2. 只 `Seek(offset)` 读新增尾部，未闭合的最后一行留到下次。
3. 每行先做**子串预过滤**（如 `"token_count"`、`inference_done`），绝大多数行不进 JSON 解码器。
4. 命令只查聚合表 `(day, model, agent)`，行数在几百量级。
5. 目录遍历带前缀剪枝：`*/logs/transcript_full.jsonl` 这类模式不会钻进 `chunks/` 之类的子树。

### 正确性自检

增量不等于近似，代理明细也不是估算：

- **本地日志**：把 mtok 某天的 Grok 聚合与该 agent 日志逐条求和对比，四个数字完全一致（`calls=926 / input=11133549 / output=1259176 / cache=175827328`）。
- **代理明细**：`mtok doctor` 会拿 journal 聚合与 `/v1/magpie/usage` 的 `models[]` 求和逐字段比对，today / 7d / 30d 三个窗口都是 **+0.0%**（见上文「看穿 magpie」）。


## 架构

```
                 ┌──────────── adapters ────────────┐
 本机日志 ──glob──▶│ 内置 Go adapter（claude/codex/…）│──Record──┐
 magpie journal ──▶│ magpie adapter（served 真实模型）│          │
                 │ 声明式 adapter（adapters.d/*.yaml）│          │
                 └──────────────────────────────────┘          ▼
                                                       ┌──────────────┐
 指向同一文件的 offset/size/mtime ─────────────────────▶│  ingest 增量 │
                                                       └──────┬───────┘
                                                              ▼
                                        ┌───────────────────────────────┐
                                        │ 聚合库 rows(day,model,agent,  │
                                        │  source, routed)  单文件快照   │
                                        └──────────────┬────────────────┘
                                                       ▼
                        agg：route 记录按 (day, agent) 被代理明细替代
                                                       ▼
                            归一化模型名 → 取价 → 渲染（CJK 列宽 / 颜色 / JSON）
```

目录：

```
cmd/mtok/                 入口（版本号注入点）
internal/adapters/        Adapter 接口、9 个内置实现、声明式引擎
internal/agg/             聚合与去重：route 记录 ←→ 代理明细的覆盖规则
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

- `config.yaml`：`width`、`top`、`currency`、`rate`、`magpie`、`magpie_url`、`magpie_usage_file`、`zstd`、`agents`（按名禁用）、`pricing_file`。
- `aliases.yaml`：`原始名或任意写法: 统一名`，覆盖内置别名表。
- `pricing.json`：与旧 `mtok.ps1` 同构（`currency.rate` / `default` / `models`），额外支持 `cache_read`、`cache_write`。
- `adapters.d/*.yaml`：声明式数据源，见 [docs/adapters.md](docs/adapters.md)。

环境变量：`MTOK_CONFIG_DIR`、`MTOK_DATA_DIR`、`MTOK_DB`（便于隔离测试）、`MAGPIE_USAGE_FILE`、`NO_COLOR`。

## 与 magpie 的关系

magpie 是**可选但强烈推荐**的增强，分三块，任何一块单独去掉都不影响其它：

| 能力 | 数据来源 | 关掉之后 |
|---|---|---|
| 代理侧模型明细（看穿 auto/group） | `~/.config/magpie/usage.jsonl` | route 记录保留为 `Auto(未指明)` |
| 剩余额度显示 | `GET /v1/magpie/quotas` | 只是少一段额度 |
| 逐字段对账 | `GET /v1/magpie/usage?period=…` | 只是 `doctor` 少一段核对 |

mtok 对 magpie 只做**只读**：不写配置、不重启、不代理请求。`--no-magpie` 一次性关闭以上三项（纯本地模式）；`config.yaml` 里 `magpie: false` 则是永久关闭。

明细里的 `served` 已经是真实底层模型，所以 `auto` 不再需要人工映射。只有当 journal 没有覆盖到某一天（或明细里 `served` 本身也是 `auto`）时，才会出现 `Auto(未指明)`，可在 `aliases.yaml` 里兜底映射：

```yaml
"magpie/auto/auto": "DeepSeek-V4.1-Flash"
```

## 二次开发

- 声明式（零编译）：`~/.config/mtok/adapters.d/*.yaml`，支持 JSONL 点路径、数组下标、正则三种取数方式。
- Go 接口（走 PR）：`internal/adapters` 里实现 `Name/Agent/Globs/ParseLine`，可选实现 `Needles()` 预过滤。

详见 [docs/adapters.md](docs/adapters.md)。

## 已知限制

- **明细只覆盖 journal 里有的日子**：magpie 的 `usage.jsonl` 只保留最近一段（本机约两周）。更早的 route 记录没有明细可还原，会保留为 `Auto(未指明)`——这是「不丢数据」的兜底，不是估算。
- **代理明细的 calls 比 magpie 少一点**：mtok 不计 0-token 记录（错误/取消），magpie 仍计次；token 三列一致。
- **route 判定靠名字形态**：`auto/*`、`group/*`、`magpie/*`、`auto-*` 视为路由；`workbuddy/deepseek-v4.1-flash` 这类带厂商前缀的真实模型名算直连。若某天它同时被代理和直连使用，会以「直连」计（极少，约 4% 的 claude 记录）。
- Grok Build 的 `unified.jsonl` 只保留当前会话；该 agent 的用量现在主要来自代理明细，本地日志只是补充。
- Kimi 的日志不含逐请求模型名，只能用 `config.toml` 的 `default_model`，`doctor` 里会标出。
- OpenCode / OpenClaw 的本地库是 SQLite（未解析），但它们经代理的用量已由明细覆盖。
- 日志被截断/轮转时 offset 归零重读，历史贡献无法撤回：同步行与 `doctor` 会提示，必要时 `mtok --rebuild`。

## 与 ccusage 的关系

解析思路（各家日志的位置与字段形态）参考了 ccusage 这类开源工具的经验，但 mtok 不含、不调用、不依赖 ccusage，全部字段均按本机样本重新确认。

## 文档与贡献

- [统计口径](docs/caliber.md)
- [数据源与 adapter](docs/adapters.md)
- [开发规划](docs/ROADMAP.md)
- [贡献指南](CONTRIBUTING.md)
- [安全说明](SECURITY.md)

## License

MIT

