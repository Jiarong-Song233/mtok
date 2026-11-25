# mtok 统计口径

mtok 的目标是把不同 agent 日志里的 token 数字整理成一套可以相加的账。所有字段都按本机真实日志确认；字段缺失时明确标为「暂不支持」，不猜测、不估算。

## token 字段

| 字段 | 定义 | 备注 |
|---|---|---|
| 输入 | 本次请求中未命中缓存的 prompt token | 与厂商的 `input_tokens` 不一定同义，按下表折算 |
| 输出 | 厂商上报的 completion / output token | 包含 reasoning token；Codex 的 `reasoning_output_tokens` 已包含在 `output_tokens` 内 |
| 缓存 | 缓存读（命中 prompt cache）+ 缓存写（写入 prompt cache） | 报表里合并成一列显示；计价时仍按 cache read / cache write 两个单价分开算 |
| 总和 | 输入 + 输出 + 缓存读 + 缓存写 | 表格里的「总和」列 |

金额公式：

```text
cost = input/1e6 * input_price
     + output/1e6 * output_price
     + cache_read/1e6 * cache_read_price
     + cache_write/1e6 * cache_write_price
```

价格表单位是 USD / 百万 token。默认币种是 USD；`config.yaml` 里设 `currency: CNY` 时，用 `rate` 把 USD 换算成 CNY。金额是参考价，不是账单。

## 各来源映射

| 来源 | 输入 | 缓存读 | 缓存写 | 说明 |
|---|---|---|---|---|
| Claude Code | `input_tokens` | `cache_read_input_tokens` | `cache_creation_input_tokens` | Anthropic 的 `input_tokens` 本身不含缓存 |
| Codex | `input_tokens - cached_prompt_tokens` | `cached_prompt_tokens` | 日志未区分 | Codex 的 prompt 字段含缓存命中，mtok 折算到缓存读 |
| Gemini / Antigravity | `input_tokens` | `cache_read_tokens` | 日志未区分 | 模型名来自 `Model Selection` 变更行 |
| Grok Build | `prompt_tokens - cached_prompt_tokens` | `cached_prompt_tokens` | 日志未区分 | 输出取 `completion_tokens` |
| Kimi | `input_other` | `input_cache_read` | `input_cache_creation` | wire 日志不含逐请求模型名，用配置里的默认模型 |
| ZCode | `inputTokens` | `cacheReadTokens` | `cacheWriteTokens` | 字段同名对应 |
| pi | `input` | `cacheRead` | `cacheWrite` | 字段同名对应 |
| dsh | `data.usage` 的输入字段 | `data.usage` 的缓存读字段 | `data.usage` 的缓存写字段 | zstd 归档整包解码一次 |
| magpie journal | `in` | `cache_read` | `cache_write` | `served` 是真实底层模型，用来看穿路由 |

LiteLLM 价格表里 Anthropic 的 `cache_creation_input_token_cost` 会映射为 cache write 单价。

## 去重与路由

magpie 的调用日志按 `(day, agent)` 覆盖同日的 route 记录。route 记录指模型名形如 `auto/*`、`group/*`、`magpie/*`、`auto-*` 的本地日志，它们不带真实模型身份，不能和明细同时计入。

- 某天某 agent 有明细：该 agent 当天的 route 记录整段被明细替代。
- 明细没有覆盖的日子：route 记录保留为 `Auto(未指明)`，避免丢数据。
- 直连日志照常计入，例如 Claude 直连 Anthropic、Codex 直连 OpenAI、Kimi、ZCode。
- `--no-magpie` 关闭代理明细、代理额度和对账，只保留本地来源。

`mtok doctor` 会把 journal 聚合与 magpie 的 `/v1/magpie/usage` 做逐字段对账。token 三列应一致；`calls` 可能略少，因为 mtok 不计 0-token 的错误或取消记录。

## 时间归属

记录按时间戳换算到本机时区的日期。没有时间戳的极少数记录计入扫描当天，`mtok doctor` 会报出条数。指定日期、`7d`、`month` 等窗口都按本机日历日切分。

## 额度

额度数据来自本机文件与本地 magpie 接口，全程离线，不调用厂商私有 API：

- Codex：读取 `~/.codex/sessions/**/rollout-*.jsonl[.zst]` 里的 `rate_limits.primary/secondary`，显示 5h / 7d 剩余百分比和重置时间。
- T3：读取 `~/.t3/caches/*.json` 里各 agent 的 `usageLimits.windows`，显示 Gemini、Claude Code、Grok 等的 5 小时/周剩余百分比和重置时间。Codex 的 T3 缓存会被跳过，避免与更权威的会话日志读数重复。
- magpie：读取 `GET /v1/magpie/quotas`；同名 provider 以本地读数优先。
- T3 未覆盖的 agent 才提示用各自 CLI 的 `/status` 查看。

## 验证新来源

新增 adapter 时必须同时完成：

1. 在 `docs/caliber.md` 写清四个 token 字段的映射和理由。
2. 加解析测试，覆盖正常行、缺字段行、缓存字段行。
3. 能本机取到对照数据时，用 `mtok doctor` 做逐字段对账。
4. 无法读取的来源要能被 `doctor` 列出并说明原因。
