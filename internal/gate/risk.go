package gate

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// RiskLevel 定义工具操作的风险等级。
type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
)

// TrustLevel 定义用户的信任偏好。
type TrustLevel int

const (
	// TrustCautious 谨慎模式：所有 Medium/High 都需审批。
	TrustCautious TrustLevel = iota
	// TrustBalanced 平衡模式：Medium 首次审，High 始终审（默认）。
	TrustBalanced
	// TrustPermissive 宽松模式：只有 High 审，Medium 自动过。
	TrustPermissive
)

// AutoApproveEntry 记录学习到的自动审批项。
type AutoApproveEntry struct {
	Key       string
	UserID    string
	ApprovedAt time.Time
	LastUsed  time.Time
}

// learnedAutoApproveTTL 是学习到的自动审批项的过期时间。
const learnedAutoApproveTTL = 7 * 24 * time.Hour

// RiskBasedEvaluator 基于风险等级和信任偏好评估工具审批需求。
type RiskBasedEvaluator struct {
	trustLevel          TrustLevel
	learnedAutoApproves map[string]*AutoApproveEntry
	mu                  sync.RWMutex
}

// NewRiskBasedEvaluator 创建新的风险策略评估器。
func NewRiskBasedEvaluator(trust TrustLevel) *RiskBasedEvaluator {
	return &RiskBasedEvaluator{
		trustLevel:          trust,
		learnedAutoApproves: make(map[string]*AutoApproveEntry),
	}
}

// Evaluate 评估工具调用的审批需求。
func (e *RiskBasedEvaluator) Evaluate(toolName string, params map[string]any, userID string) ApprovalRequirement {
	risk := e.assessRisk(toolName, params)
	switch risk {
	case RiskLow:
		return Never
	case RiskHigh:
		return Always
	case RiskMedium:
		switch e.trustLevel {
		case TrustPermissive:
			return Never
		case TrustCautious:
			return UnlessAutoApproved
		case TrustBalanced:
			if e.isLearnedApproved(toolName, params, userID) {
				return Never
			}
			return UnlessAutoApproved
		}
	}
	return UnlessAutoApproved
}

// assessRisk 评估工具操作的风险等级。
func (e *RiskBasedEvaluator) assessRisk(toolName string, params map[string]any) RiskLevel {
	switch toolName {
	case "shell":
		return RiskHigh
	case "file":
		action, _ := params["action"].(string)
		switch action {
		case "read":
			return RiskLow
		case "write":
			return RiskMedium
		case "delete", "mkdir":
			return RiskHigh
		default:
			return RiskMedium
		}
	case "http":
		method, _ := params["method"].(string)
		if method == "GET" || method == "HEAD" {
			return RiskLow
		}
		url, _ := params["url"].(string)
		if isExternalDomain(url) {
			return RiskHigh
		}
		return RiskMedium
	case "memory":
		action, _ := params["action"].(string)
		if action == "get" {
			return RiskLow
		}
		return RiskMedium
	case "echo", "time", "json":
		return RiskLow
	default:
		return RiskMedium
	}
}

// isExternalDomain 判断 URL 是否指向外部域名（简单启发式）。
func isExternalDomain(url string) bool {
	if url == "" {
		return false
	}
	// 本地地址不算外部
	localPrefixes := []string{"localhost", "127.0.0.1", "0.0.0.0", "::1"}
	for _, prefix := range localPrefixes {
		if strings.Contains(url, prefix) {
			return false
		}
	}
	// 内部私有地址段
	privatePrefixes := []string{"192.168.", "10.", "172.16.", "172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31."}
	for _, prefix := range privatePrefixes {
		if strings.HasPrefix(url, "http://"+prefix) || strings.HasPrefix(url, "https://"+prefix) {
			return false
		}
	}
	return true
}

// autoApproveKey 生成学习模式的键。
func autoApproveKey(toolName string, params map[string]any) string {
	action := ""
	if a, ok := params["action"].(string); ok {
		action = a
	}
	return fmt.Sprintf("%s:%s", toolName, action)
}

// isLearnedApproved 检查某工具操作是否已被学习为自动审批。
func (e *RiskBasedEvaluator) isLearnedApproved(toolName string, params map[string]any, userID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := userID + "/" + autoApproveKey(toolName, params)
	entry, ok := e.learnedAutoApproves[key]
	if !ok {
		return false
	}
	if time.Since(entry.LastUsed) > learnedAutoApproveTTL {
		return false
	}
	return true
}

// RecordApproval 记录用户的一次审批，用于学习模式。
func (e *RiskBasedEvaluator) RecordApproval(toolName string, params map[string]any, userID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := userID + "/" + autoApproveKey(toolName, params)
	now := time.Now()
	e.learnedAutoApproves[key] = &AutoApproveEntry{
		Key:       key,
		UserID:    userID,
		ApprovedAt: now,
		LastUsed:  now,
	}
}

// UpdateLastUsed 更新学习项的最后使用时间。
func (e *RiskBasedEvaluator) UpdateLastUsed(toolName string, params map[string]any, userID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := userID + "/" + autoApproveKey(toolName, params)
	entry, ok := e.learnedAutoApproves[key]
	if ok {
		entry.LastUsed = time.Now()
	}
}

// CleanupExpired 清理过期的学习项。
func (e *RiskBasedEvaluator) CleanupExpired() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for key, entry := range e.learnedAutoApproves {
		if time.Since(entry.LastUsed) > learnedAutoApproveTTL {
			delete(e.learnedAutoApproves, key)
		}
	}
}

// LearnedCount 返回当前学习到的自动审批项数量。
func (e *RiskBasedEvaluator) LearnedCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.learnedAutoApproves)
}
