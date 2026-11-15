# 二次开发：新增一个数据源

mtok 的数据源分两类：

1. **声明式 adapter**：一个 YAML 文件，放进 `~/.config/mtok/adapters.d/` 就生效，**不需要编译**。
2. **Go adapter**：实现一个接口，进 `internal/adapters/`，适合复杂/高频数据源，欢迎 PR。

两者产出的都是同一个 `Record`，后续归一化、计价、渲染完全共用。

---

## 0. Record 与口径约定

```go
type Record struct {
    Time       time.Time // 必填；零值会计入扫描当天并在 doctor 里报出
    Model      string    // 原始模型名，交给归一化处理
    Agent      string    // 留空则用 adapter 的 Agent()
    Input      int64     // 未命中缓存的输入 token
    Output     int64     // 输出 token（含推理）
    CacheRead  int64
    CacheWrite int64
}
```

务必遵守「Input 不含缓存」这一条，否则跨厂商相加会重复计算。若源字段是「含缓存的 prompt tokens」，请减掉缓存部分（声明式用 `input_includes_cache: true`）。

## 1. 声明式 adapter

### 1.1 最小可用示例

`~/.config/mtok/adapters.d/myagent.yaml`：

```yaml
name: myagent
agent: myagent
glob: ~/.myagent/logs/**/*.jsonl
record:
  time:
    path: timestamp
  model:
    path: message.model
  input:
    path: usage.input_tokens
  output:
    path: usage.output_tokens
```

当天日志里若出现 `{"timestamp": "...", "message": {"model": "acme-v4.1"}, "usage": {"input_tokens": 100, "output_tokens": 20}}`，`mtok 7d` 就会多出一行 `Acme-v4.1`。

### 1.2 完整 schema

```yaml
name: myagent                # 必填，唯一 id（也是 config.yaml 里禁用它的键）
agent: myagent               # 可选，--by-agent 显示的名字
glob: ~/.myagent/logs/**/*.jsonl   # 必填，支持 ~ / 环境变量 / * ? **
globs:                       # 可选，多个路径
  - ~/.myagent/logs-archive/**/*.jsonl
disabled: false              # true 等于临时停用
format: jsonl                # jsonl（默认）或 regex
contains: '"usage"'          # 可选，单条预过滤子串（强烈建议写，省 I/O）
contains_any:                # 可选，多条任一命中即解析
  - '"usage"'
  - '"tokens"'

record:                      # jsonl 模式
  filter:                    # 全部条件满足才计数
    - path: type             # 字段路径
      equals: assistant      # 精确相等
    - path: kind
      has: response          # 子串包含
  time:
    path: timestamp          # 默认按 RFC3339 等常见格式解析
    # unix_ms: true          # 毫秒时间戳
    # unix_s: true           # 秒时间戳（可为浮点）
  model:
    path: message.model
  input:
    path: usage.input_tokens
  output:
    path: usage.output_tokens
  cache_read:
    path: usage.cache_read_input_tokens
  cache_write:
    path: usage.cache_creation_input_tokens
  input_includes_cache: true # 源字段含缓存命中时打开，mtok 会减去 cache_read(+write)
  model_from_file: true      # 模型只在某些行出现时，记住它供后续行使用
```

`time_layout`（顶层）可以为特殊时间格式指定 Go 布局，例如 `time_layout: "2006-01-02 15:04:05"`。

### 1.3 字段路径语法

- 点号进入对象：`message.usage.input_tokens`
- 方括号进数组：`messages[0].usage.input`
- 可嵌套：`choices[0].message.usage.output`

### 1.4 正则模式

适合「一行一条、不是 JSON」的日志：

```yaml
name: mylog
glob: ~/.mylog/*.log
format: regex
contains: "tokens="
regex: '^(?P<time>\S+) model=(?P<model>\S+) in=(?P<input>\d+) out=(?P<output>\d+) cache=(?P<cache_read>\d+)$'
time_layout: "2006-01-02T15:04:05Z07:00"
record:
  input_includes_cache: false   # 正则模式同样生效
```

可用的命名捕获组：`time`、`model`、`input`、`output`、`cache_read`、`cache_write`。

### 1.5 调试

```powershell
mtok adapters                 # 看是否加载、路径、prefilter
mtok doctor                   # 看命中文件数 / 本次解析记录数 / 模型 / 报错
mtok --rebuild                # 改完映射后重算
$env:MTOK_DB="$env:TEMP\mtok-test.db"; mtok all --json   # 用独立库试，不污染主库
```

常见问题：

- **文件 0 个**：`glob` 写错（Windows 路径分隔符会被自动处理，但 `~` 只认开头），或目录不存在。
- **文件有、记录 0**：`contains` 预过滤写得太严格，或 `filter.equals` 与真实值不一致。先把 `contains` 删掉试。
- **记录有、模型是 Unknown**：`model.path` 指错，或该来源本身不写模型名（用 `model_from_file` / 在 `aliases.yaml` 里补映射）。
- **数字偏大**：`input` 含缓存但没打开 `input_includes_cache`。

---

## 2. Go adapter

当数据源需要状态机、去重、特殊编码（zstd 等）时，用 Go 实现。

### 2.1 接口

```go
type Adapter interface {
    Name() string     // 唯一 id，配置里按它禁用
    Agent() string    // 表格里 --by-agent 的名字
    Globs() []string  // 绝对路径 glob，支持 **
    ParseLine(line []byte, ctx *Context) []Record
}
```

可选接口：

```go
type Prefilterer interface{ Needles() []string }  // 子串任一命中才进 ParseLine
```

`Context` 在两个方向上都可用：

```go
type Context struct {
    Path  string              // 当前文件
    Model string              // 最近一次出现的模型；可读可写
    Seen  map[string]struct{} // ctx.MarkSeen(id) 做单次扫描内去重
}
```

`Model` 会被持久化到该文件的 offset 记录里，所以**增量扫描从文件中段开始时仍然知道模型**——Codex 的 `turn_context` 就是这么工作的。

### 2.2 实现示例

`internal/adapters/myagent.go`：

```go
package adapters

type MyAgent struct{ home string }

func NewMyAgent(home string) *MyAgent { return &MyAgent{home: home} }

func (a *MyAgent) Name() string  { return "myagent" }
func (a *MyAgent) Agent() string { return "myagent" }

func (a *MyAgent) Globs() []string {
    return []string{filepath.ToSlash(filepath.Join(a.home, ".myagent", "logs", "**", "*.jsonl"))}
}

func (a *MyAgent) Needles() []string { return []string{"input_tokens"} }

func (a *MyAgent) ParseLine(line []byte, ctx *Context) []Record {
    var m map[string]json.RawMessage
    if json.Unmarshal(line, &m) != nil {
        return nil
    }
    usage := obj(m, "usage")
    if usage == nil {
        return nil
    }
    if model := str(m, "model"); model != "" {
        ctx.Model = model
    }
    t, _ := timeFrom(m, "timestamp")
    return []Record{{
        Time:      t,
        Model:     ctx.Model,
        Input:     i64(usage, "input_tokens"),
        Output:    i64(usage, "output_tokens"),
        CacheRead: i64(usage, "cached_tokens"),
    }}
}
```

注册到 `internal/adapters/registry.go` 的 `Builtins()` 列表，然后：

```go
func TestMyAgentParseLine(t *testing.T) {
    a := NewMyAgent(t.TempDir())
    ctx := &Context{}
    recs := a.ParseLine([]byte(`{"timestamp":"2026-11-15T01:02:03Z","model":"acme-v41","usage":{"input_tokens":10,"output_tokens":2}}`), ctx)
    if len(recs) != 1 || recs[0].Input != 10 || recs[0].Model != "acme-v41" {
        t.Fatalf("unexpected %+v", recs)
    }
}
```

可复用的小工具都在 `internal/adapters/adapter.go` 与 `util.go`：`str/obj/i64/f64/timeFrom/ParseTime`、`lookup/strAt/intAt`（点路径取值）。

若来源是压缩归档（`*.zst`），不用做任何事：`internal/ingest` 会按扩展名整包解码，并把该文件标记为「只解析一次」，避免重复计数。

### 2.3 测试清单

```powershell
go test ./...
go vet ./...
gofmt -l internal cmd
```

请至少覆盖：

- 一条真实样本行 → 期望的 `Record`；
- 应当被忽略的行（别的记录类型、`<synthetic>`、重复 id）；
- 状态跨行的情况（模型从前一行继承）；
- 若实现了 `Needles()`，一条应当被过滤掉的行。

### 2.4 归一化与价格

新增模型名请在 `internal/model/aliases.go` 里补 `统一名: {各种写法}`，在 `internal/pricing/builtin.json` 里补单价（USD / 百万 token）。别名表有单元测试防止同一写法被两个统一名抢占。

## 3. 提交 PR

- 一个数据源一个 PR：adapter + 测试 + 文档表格一行（README 的「数据源」）。
- 不要引入 cgo、不要引入新的重量级依赖（当前只有 `klauspost/compress` 与 `yaml.v3`）。
- 提交前 `scripts/build.ps1 -Targets windows/amd64,linux/amd64,darwin/arm64` 能全部通过。
