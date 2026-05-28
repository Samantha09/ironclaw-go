# Safety Layer 模块化重构设计

**日期**: 2026-05-28
**作者**: Claude
**状态**: 待审查

## 背景

当前 `internal/safety/safety.go` 为 127 行的单文件实现，仅提供基础的关键词匹配、简单正则和速率限制。对照 Rust 原版 `ironclaw_safety` crate（~180K 行，含泄漏检测、策略引擎、输入验证、净化器等），Go 版本的安全管道严重不足，难以支撑生产环境的安全需求。

## 目标

1. 建立可扩展、可测试的安全管道架构，对齐 Rust 原版设计
2. 保持现有 `Layer` facade 接口向后兼容，不影响调用方
3. 引入可配置的策略规则引擎，支持 severity/action 分级响应
4. 增强 secret 泄漏检测与脱敏能力
5. 每个子包独立可测，覆盖已知攻击模式和边界情况

## 架构

将 `internal/safety` 拆分为 5 个子包 + 2 个顶层文件：

```
internal/safety/
├── layer.go              # 统一 facade，保持现有接口兼容
├── config.go             # SafetyConfig 配置
├── ratelimit.go          # 速率限制器（从现有代码提取）
├── validator/            # 输入验证
│   ├── validator.go      # 核心验证器
│   └── result.go         # ValidationResult / ValidationError
├── sanitizer/            # 输出净化
│   ├── sanitizer.go      # 工具输出净化、长度截断
│   └── output.go         # SanitizedOutput / InjectionWarning
├── policy/               # 策略引擎
│   ├── policy.go         # Policy 规则集合
│   └── rule.go           # Rule / Severity / Action / Violation
└── leakdetector/         # 秘密泄漏检测
    ├── detector.go       # 扫描与清除
    └── patterns.go       # 预定义检测模式
```

### 职责边界

| 包 | 职责 | 不做什么 |
|---|---|---|
| `validator` | 判断输入是否合法，返回错误/警告 | 不修改内容 |
| `sanitizer` | 修改/净化内容，标记修改位置 | 不阻塞流程 |
| `policy` | 提供可配置规则引擎 | 不直接处理 I/O |
| `leakdetector` | 检测 secrets/credentials 并脱敏 | 不做语义分析 |
| `layer` | 统一调度，保持对外接口稳定 | 不包含业务逻辑 |

## 组件接口

### policy

```go
type Severity int
const (
    Low Severity = iota + 1
    Medium
    High
    Critical
)

type Action int
const (
    Flag Action = iota
    Block
    Sanitize
)

type Rule struct {
    ID          string
    Description string
    Pattern     *regexp.Regexp
    Severity    Severity
    Action      Action
}

type Violation struct {
    RuleID      string
    Description string
    Severity    Severity
    Action      Action
    MatchedText string
}

type Policy struct{ rules []Rule }

func (p *Policy) AddRule(r Rule)
func (p *Policy) Evaluate(content string) []Violation
func (p *Policy) LoadDefaults()
```

### validator

```go
type ErrorCode string
const (
    ErrForbiddenContent  ErrorCode = "forbidden_content"
    ErrSuspiciousPattern ErrorCode = "suspicious_pattern"
    ErrTooLong           ErrorCode = "too_long"
    ErrInvalidEncoding   ErrorCode = "invalid_encoding"
)

type ValidationError struct {
    Field   string
    Message string
    Code    ErrorCode
}

type Result struct {
    IsValid  bool
    Errors   []ValidationError
    Warnings []string
}

type Validator struct{ policy *policy.Policy }

func (v *Validator) ValidateInput(ctx context.Context, content string) Result
func (v *Validator) ScanCode(ctx context.Context, code string) Result
```

### sanitizer

```go
type Warning struct {
    Pattern     string
    Severity    policy.Severity
    Location    [2]int
    Description string
}

type Output struct {
    Content     string
    Warnings    []Warning
    WasModified bool
}

type Sanitizer struct{ policy *policy.Policy }

func (s *Sanitizer) SanitizeToolOutput(ctx context.Context, toolName, content string, maxLen int) Output
```

### leakdetector

```go
type Match struct {
    Pattern string
    Type    string // "api_key", "password", "token", "private_key"
}

type Detector struct{ patterns []PatternInfo }

func (d *Detector) Scan(content string) []Match
func (d *Detector) ScanAndClean(content string) (string, []Match)
```

### layer（统一 facade）

```go
type Config struct {
    MaxOutputLength int
    RateMaxCalls    int
    RateWindow      time.Duration
}

type Layer struct {
    validator    *validator.Validator
    sanitizer    *sanitizer.Sanitizer
    policy       *policy.Policy
    leakDetector *leakdetector.Detector
    rateLimiter  *RateLimiter
    config       Config
}

func NewLayer(config Config) *Layer
func (l *Layer) ScanInbound(ctx context.Context, content string) error
func (l *Layer) SanitizeToolOutput(ctx context.Context, toolName, content string) (string, error)
func (l *Layer) ScanCode(ctx context.Context, code string) error
func (l *Layer) Allow(key string) bool
```

## 数据流

### 输入路径（用户消息 → LLM）

```
ScanInbound(content)
  → validator.ValidateInput(content)
    → policy.Evaluate(content)
      → Critical/High + Block → 返回 ValidationError → ScanInbound 返回 error
      → Medium/Low + Flag     → 记录警告，继续
    → 检测 XML 标签注入、编码异常
  → 通过则返回 nil
```

### 输出路径（工具输出 → LLM）

```
SanitizeToolOutput(toolName, content)
  → sanitizer.Sanitize(toolName, content, maxLen)
    → 长度检查：超限则截断并附加提示
    → policy.Evaluate(content)
  → leakdetector.ScanAndClean(content)
    → 发现 secrets → 替换为 [REDACTED]，记录 Match
  → 返回净化后字符串
```

### 代码路径

```
ScanCode(code)
  → validator.ScanCode(code)
    → 检测 eval/exec/system/subprocess 等危险调用
    → policy.Evaluate(code)
  → 发现则返回 error，否则 nil
```

## 错误处理

- **Critical / High + Block**: 返回 `internal/errors` 中的 `SafetyError`，阻断处理
- **Medium / Low + Flag**: 记录到 `ValidationResult.Warnings`，不阻塞流程
- **SanitizeToolOutput**: 不返回 error，返回净化内容 + 警告；若泄漏检测失败则返回占位文本 `[Output blocked due to potential secret leakage]`

## 测试策略

每个子包独立测试，表驱动：

| 测试文件 | 覆盖内容 |
|---|---|
| `policy_test.go` | 规则匹配、严重级别排序、默认规则加载 |
| `validator_test.go` | 已知注入模式、边界长度、编码异常、误报控制 |
| `sanitizer_test.go` | 长度截断、秘密脱敏、多轮修改标记 |
| `leakdetector_test.go` | API key、密码、token、私钥的正反例 |
| `layer_test.go` | 端到端集成，验证 facade 接口行为与重构前一致 |

## 范围与排除项

**在范围内：**
- 子包拆分与接口定义
- 策略规则引擎（severity/action）
- Secret 泄漏检测增强（API key、密码、token、私钥）
- Prompt injection 检测增强（更多模式、XML 注入）
- 长度截断与提示
- 现有接口向后兼容

**不在范围内：**
- Fuzz 测试（后续迭代）
- 敏感路径检测（当前无对应使用场景）
- 凭证注入检测（当前无对应使用场景）
- 外部策略服务/动态规则热加载（后续迭代）

## 风险与回滚

- **风险**: 重构可能引入接口不兼容。缓解：`layer_test.go` 验证 facade 行为与重构前一致。
- **回滚**: 若出现问题，可回退到 `safety.go` 单文件版本，不影响外部调用方。

## 参考

- Rust 原版: `/home/san/GolandProjects/ironclaw/crates/ironclaw_safety/`
- 当前实现: `/home/san/GolandProjects/ironclaw-go/internal/safety/safety.go`
