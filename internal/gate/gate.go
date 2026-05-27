// Package gate 提供工具调用执行门控，实现交互式审批和自主执行模式。
package gate

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ApprovalRequirement 定义工具调用的审批需求级别。
type ApprovalRequirement int

const (
	// Never 无需审批。
	Never ApprovalRequirement = iota
	// UnlessAutoApproved 需要审批，但会话级自动审批可绕过。
	UnlessAutoApproved
	// Always 始终需要显式审批（即使已设置自动审批）。
	Always
)

// IsRequired 返回此调用是否需要审批（在自动审批不相关的情境中）。
func (a ApprovalRequirement) IsRequired() bool {
	return a != Never
}

// ExecutionMode 定义执行情境。
type ExecutionMode int

const (
	// Interactive 交互式会话，可暂停等待用户确认。
	Interactive ExecutionMode = iota
	// Autonomous 自主模式（后台任务），无法暂停，拒绝需审批的操作。
	Autonomous
)

// GateDecision 是门控评估结果。
type GateDecision int

const (
	// Allow 允许执行。
	Allow GateDecision = iota
	// Deny 拒绝执行。
	Deny
	// Pause 暂停执行，等待用户审批。
	Pause
)

// GateContext 提供门控评估所需的上下文。
type GateContext struct {
	ToolName       string
	Params         map[string]any
	UserID         string
	ThreadID       string
	AutoApproved   map[string]bool // 已自动审批的工具集合
	ExecutionMode  ExecutionMode
	Channel        string
}

// Gate 是执行门控接口，在工具调度前评估是否允许执行。
type Gate interface {
	Name() string
	Evaluate(ctx context.Context, gctx *GateContext) GateDecision
}

// PendingGate 表示一个暂停等待审批的门控状态。
type PendingGate struct {
	RequestID     string
	GateName      string
	UserID        string
	ThreadID      string
	ToolName      string
	Params        map[string]any
	Description   string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	SourceChannel string
}

// IsExpired 检查门控是否已过期（默认 10 分钟）。
func (p *PendingGate) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// PauseError 表示门控暂停，需要用户审批。
type PauseError struct {
	ToolName    string
	RequestID   string
	Description string
}

func (e *PauseError) Error() string {
	return fmt.Sprintf("tool '%s' requires approval (request %s): %s", e.ToolName, e.RequestID, e.Description)
}

// IsPauseError 检查错误是否是 PauseError。
func IsPauseError(err error) bool {
	var pe *PauseError
	return errors.As(err, &pe)
}

// GateError 是门控相关的错误。
type GateError struct {
	Reason string
}

func (e *GateError) Error() string {
	return fmt.Sprintf("gate: %s", e.Reason)
}
