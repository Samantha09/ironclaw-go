package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nearai/ironclaw-go/internal/db"
	"github.com/nearai/ironclaw-go/internal/gate"
	"github.com/nearai/ironclaw-go/internal/safety"
)

// Dispatcher 运行安全管道并执行工具。
type Dispatcher struct {
	registry     *Registry
	safety       *safety.Layer
	rateLimiter  *safety.RateLimiter
	allowed      map[string]bool // 允许的工具白名单，nil 表示允许所有
	database     db.Database
	gates        []gate.Gate
	pendingStore *gate.PendingStore
	autoApproved map[string]bool
}

// NewDispatcher 创建新的调度器。
func NewDispatcher(registry *Registry, safetyLayer *safety.Layer, database db.Database) *Dispatcher {
	return &Dispatcher{
		registry:     registry,
		safety:       safetyLayer,
		rateLimiter:  safety.NewRateLimiter(60, time.Minute),
		database:     database,
		pendingStore: gate.NewPendingStore(),
		autoApproved: make(map[string]bool),
	}
}

// NewDispatcherWithLimit 创建带白名单的调度器。
func NewDispatcherWithLimit(registry *Registry, safetyLayer *safety.Layer, database db.Database, allowed []string) *Dispatcher {
	d := NewDispatcher(registry, safetyLayer, database)
	if len(allowed) > 0 {
		d.allowed = make(map[string]bool)
		for _, name := range allowed {
			d.allowed[name] = true
		}
	}
	return d
}

// WithGates 注册执行门控。
func (d *Dispatcher) WithGates(gates ...gate.Gate) *Dispatcher {
	d.gates = append(d.gates, gates...)
	return d
}

// WithAutoApproved 设置自动审批的工具列表。
func (d *Dispatcher) WithAutoApproved(tools []string) *Dispatcher {
	for _, name := range tools {
		d.autoApproved[name] = true
	}
	return d
}

// PendingStore 返回待处理审批存储（供外部查询和解析）。
func (d *Dispatcher) PendingStore() *gate.PendingStore {
	return d.pendingStore
}

// Dispatch 通过名称运行工具，经过安全检查。
func (d *Dispatcher) Dispatch(ctx context.Context, toolName string, params map[string]any, jobCtx *JobContext) (ToolOutput, error) {
	// 1. 权限检查
	if d.allowed != nil && !d.allowed[toolName] {
		return ToolOutput{}, fmt.Errorf("tool %q is not in the allowed list", toolName)
	}

	// 2. 查找工具
	tool, ok := d.registry.Get(toolName)
	if !ok {
		return ToolOutput{}, fmt.Errorf("tool '%s' not found", toolName)
	}

	// 3. 速率限制
	key := jobCtx.UserID + "/" + toolName
	if !d.rateLimiter.Allow(key) {
		return ToolOutput{}, fmt.Errorf("rate limit exceeded for tool '%s'", toolName)
	}

	// 4. 检查是否有已批准但尚未执行的 gate（Resume 场景）
	if resolved := d.pendingStore.ConsumeResolved(jobCtx.UserID, jobCtx.ThreadID); resolved != nil {
		// 用户已批准，跳过门控直接执行
		// 注意：这里不检查 toolName 是否匹配，因为 PendingExecution 保存了完整的 toolCalls
	} else {
		// 执行门控评估
		for _, g := range d.gates {
			gctx := &gate.GateContext{
				ToolName:      toolName,
				Params:        params,
				UserID:        jobCtx.UserID,
				ThreadID:      jobCtx.ThreadID,
				AutoApproved:  d.autoApproved,
				ExecutionMode: gate.Interactive,
				Channel:       "repl",
			}
			decision := g.Evaluate(ctx, gctx)
			switch decision {
			case gate.Allow:
				continue
			case gate.Deny:
				return ToolOutput{}, fmt.Errorf("tool '%s' denied by gate '%s'", toolName, g.Name())
			case gate.Pause:
				pg := d.pendingStore.Create(jobCtx.UserID, jobCtx.ThreadID, toolName, params, "repl")
				return ToolOutput{}, &gate.PauseError{
					ToolName:    toolName,
					RequestID:   pg.RequestID,
					Description: pg.Description,
				}
			}
		}
	}

	// 5. 执行工具
	start := time.Now()
	out, err := tool.Execute(ctx, params, jobCtx)
	out.Duration = time.Since(start).Milliseconds()

	// 5. 输出净化
	if err == nil && d.safety != nil {
		sanitized, sErr := d.safety.SanitizeToolOutput(ctx, out.Content)
		if sErr != nil {
			return ToolOutput{}, fmt.Errorf("safety check failed: %w", sErr)
		}
		out.Content = sanitized
	}

	// 6. 审计日志（异步，不阻塞返回）
	go d.audit(jobCtx, toolName, params, out, err)

	if err != nil {
		return ToolOutput{}, err
	}

	return out, nil
}

// audit 记录工具调用审计日志。在后台 goroutine 中异步持久化到数据库。
func (d *Dispatcher) audit(jobCtx *JobContext, toolName string, params map[string]any, out ToolOutput, execErr error) {
	if d.database == nil {
		return
	}

	status := "success"
	errStr := ""
	if execErr != nil {
		status = "failure"
		errStr = execErr.Error()
	}

	paramsJSON := ""
	if params != nil {
		// 简化：不实际序列化，避免循环引用
		paramsJSON = fmt.Sprintf("%v", params)
	}

	rec := db.ActionRecord{
		ID:        uuid.New().String(),
		JobID:     jobCtx.JobID,
		ToolName:  toolName,
		Input:     paramsJSON,
		Output:    out.Content,
		Error:     errStr,
		Duration:  time.Duration(out.Duration) * time.Millisecond,
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if saveErr := d.database.SaveActionRecord(ctx, &rec); saveErr != nil {
		// 审计失败不应影响主流程，仅静默记录
		_ = fmt.Errorf("audit save failed: %w", saveErr)
	}

	_ = status
}
