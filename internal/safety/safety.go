package safety

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Layer 提供输入扫描和输出净化。
type Layer struct {
	mu             sync.RWMutex
	blockedWords   []string
	secretPatterns []*regexp.Regexp
}

// NewLayer 创建新的安全层。
func NewLayer() *Layer {
	return &Layer{
		blockedWords: []string{
			"ignore previous instructions",
			"disregard all prior",
			"you are now",
			"system prompt",
			"DAN mode",
			"jailbreak",
		},
		secretPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*['"]?[a-zA-Z0-9_-]{20,}['"]?`),
			regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*['"]?[^\s'"]+['"]?`),
			regexp.MustCompile(`(?i)(token|secret)\s*[:=]\s*['"]?[a-zA-Z0-9_-]{16,}['"]?`),
			regexp.MustCompile(`\b(sk-[a-zA-Z0-9]{20,})\b`),
		},
	}
}

// ScanInbound 检测用户输入中的 prompt injection 和恶意内容。
func (l *Layer) ScanInbound(_ context.Context, content string) error {
	lower := strings.ToLower(content)

	// 检测 prompt injection 关键词
	for _, word := range l.blockedWords {
		if strings.Contains(lower, strings.ToLower(word)) {
			return fmt.Errorf("safety: detected suspicious pattern: %q", word)
		}
	}

	// 检测 XML 标签注入（常见的越狱手段）
	if strings.Contains(content, "</") && strings.Contains(content, ">") {
		if strings.Contains(content, "system") || strings.Contains(content, "instructions") {
			return fmt.Errorf("safety: detected potential XML injection")
		}
	}

	return nil
}

// SanitizeToolOutput 检测并净化工具输出中的敏感信息。
func (l *Layer) SanitizeToolOutput(_ context.Context, content string) (string, error) {
	l.mu.RLock()
	patterns := l.secretPatterns
	l.mu.RUnlock()

	sanitized := content
	for _, re := range patterns {
		sanitized = re.ReplaceAllString(sanitized, "[REDACTED]")
	}

	return sanitized, nil
}

// ScanCode 检测代码中的危险操作（如 eval、exec 等）。
func (l *Layer) ScanCode(_ context.Context, code string) error {
	dangerous := []string{"eval(", "exec(", "system(", "os.system", "subprocess.call"}
	lower := strings.ToLower(code)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return fmt.Errorf("safety: detected dangerous code pattern: %q", d)
		}
	}
	return nil
}

// RateLimiter 提供简单的速率限制。
type RateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*rateLimit
	maxCalls int
	window   time.Duration
}

type rateLimit struct {
	count  int
	window time.Time
}

// NewRateLimiter 创建速率限制器。
func NewRateLimiter(maxCalls int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits:   make(map[string]*rateLimit),
		maxCalls: maxCalls,
		window:   window,
	}
}

// Allow 检查给定 key 是否允许执行。
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	lim, ok := r.limits[key]
	if !ok || (r.window > 0 && now.After(lim.window)) {
		r.limits[key] = &rateLimit{count: 1, window: now.Add(r.window)}
		return true
	}

	if lim.count >= r.maxCalls {
		return false
	}

	lim.count++
	return true
}
