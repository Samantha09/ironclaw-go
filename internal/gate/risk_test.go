package gate

import (
	"testing"
	"time"
)

func TestAssessRisk(t *testing.T) {
	eval := NewRiskBasedEvaluator(TrustBalanced)

	cases := []struct {
		tool   string
		params map[string]any
		want   RiskLevel
	}{
		{"echo", nil, RiskLow},
		{"time", nil, RiskLow},
		{"json", nil, RiskLow},
		{"memory", map[string]any{"action": "get"}, RiskLow},
		{"memory", map[string]any{"action": "set"}, RiskMedium},
		{"file", map[string]any{"action": "read"}, RiskLow},
		{"file", map[string]any{"action": "write"}, RiskMedium},
		{"file", map[string]any{"action": "delete"}, RiskHigh},
		{"file", map[string]any{"action": "mkdir"}, RiskHigh},
		{"http", map[string]any{"method": "GET"}, RiskLow},
		{"http", map[string]any{"method": "HEAD"}, RiskLow},
		{"http", map[string]any{"method": "POST", "url": "https://api.example.com"}, RiskHigh},
		{"http", map[string]any{"method": "POST", "url": "http://localhost:8080"}, RiskMedium},
		{"http", map[string]any{"method": "PUT", "url": "https://example.com"}, RiskHigh},
		{"shell", nil, RiskHigh},
		{"unknown", nil, RiskMedium},
	}

	for _, tc := range cases {
		got := eval.assessRisk(tc.tool, tc.params)
		if got != tc.want {
			t.Errorf("assessRisk(%q, %v) = %v, want %v", tc.tool, tc.params, got, tc.want)
		}
	}
}

func TestRiskBasedEvaluatorTrustLevels(t *testing.T) {
	cases := []struct {
		trust    TrustLevel
		tool     string
		params   map[string]any
		userID   string
		want     ApprovalRequirement
	}{
		// Low risk: always Never regardless of trust level
		{TrustCautious, "echo", nil, "u1", Never},
		{TrustBalanced, "echo", nil, "u1", Never},
		{TrustPermissive, "echo", nil, "u1", Never},
		{TrustCautious, "file", map[string]any{"action": "read"}, "u1", Never},

		// High risk: always Always regardless of trust level
		{TrustCautious, "shell", nil, "u1", Always},
		{TrustBalanced, "shell", nil, "u1", Always},
		{TrustPermissive, "shell", nil, "u1", Always},
		{TrustPermissive, "file", map[string]any{"action": "delete"}, "u1", Always},

		// Medium risk varies by trust level
		{TrustCautious, "file", map[string]any{"action": "write"}, "u1", UnlessAutoApproved},
		{TrustBalanced, "file", map[string]any{"action": "write"}, "u1", UnlessAutoApproved},
		{TrustPermissive, "file", map[string]any{"action": "write"}, "u1", Never},
		{TrustCautious, "memory", map[string]any{"action": "set"}, "u1", UnlessAutoApproved},
		{TrustPermissive, "memory", map[string]any{"action": "set"}, "u1", Never},
	}

	for _, tc := range cases {
		eval := NewRiskBasedEvaluator(tc.trust)
		got := eval.Evaluate(tc.tool, tc.params, tc.userID)
		if got != tc.want {
			t.Errorf("Evaluate(trust=%d, %q, %v) = %v, want %v", tc.trust, tc.tool, tc.params, got, tc.want)
		}
	}
}

func TestRiskBasedEvaluatorLearning(t *testing.T) {
	eval := NewRiskBasedEvaluator(TrustBalanced)
	userID := "user-1"

	// Before learning: Medium risk requires approval
	req := eval.Evaluate("file", map[string]any{"action": "write"}, userID)
	if req != UnlessAutoApproved {
		t.Errorf("before learning: got %v, want UnlessAutoApproved", req)
	}

	// Record approval
	eval.RecordApproval("file", map[string]any{"action": "write"}, userID)

	// After learning: should be Never
	req = eval.Evaluate("file", map[string]any{"action": "write"}, userID)
	if req != Never {
		t.Errorf("after learning: got %v, want Never", req)
	}

	// Different user should not benefit from learning
	req = eval.Evaluate("file", map[string]any{"action": "write"}, "user-2")
	if req != UnlessAutoApproved {
		t.Errorf("different user: got %v, want UnlessAutoApproved", req)
	}

	// Different action should not benefit
	req = eval.Evaluate("file", map[string]any{"action": "write", "path": "/tmp/x"}, "user-2")
	if req != UnlessAutoApproved {
		t.Errorf("different user still UnlessAutoApproved: got %v", req)
	}
}

func TestRiskBasedEvaluatorLearningExpiration(t *testing.T) {
	eval := NewRiskBasedEvaluator(TrustBalanced)
	userID := "user-1"

	// Manually insert an expired entry
	eval.mu.Lock()
	key := userID + "/" + autoApproveKey("file", map[string]any{"action": "write"})
	eval.learnedAutoApproves[key] = &AutoApproveEntry{
		Key:       key,
		UserID:    userID,
		ApprovedAt: time.Now().Add(-10 * 24 * time.Hour),
		LastUsed:  time.Now().Add(-10 * 24 * time.Hour),
	}
	eval.mu.Unlock()

	// Should not be approved because expired
	req := eval.Evaluate("file", map[string]any{"action": "write"}, userID)
	if req != UnlessAutoApproved {
		t.Errorf("expired learning: got %v, want UnlessAutoApproved", req)
	}
}

func TestRiskBasedEvaluatorCleanupExpired(t *testing.T) {
	eval := NewRiskBasedEvaluator(TrustBalanced)
	userID := "user-1"

	// Insert one fresh and one expired entry
	eval.mu.Lock()
	eval.learnedAutoApproves["user-1/file:write"] = &AutoApproveEntry{
		Key:       "user-1/file:write",
		UserID:    userID,
		ApprovedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	eval.learnedAutoApproves["user-1/memory:set"] = &AutoApproveEntry{
		Key:       "user-1/memory:set",
		UserID:    userID,
		ApprovedAt: time.Now().Add(-10 * 24 * time.Hour),
		LastUsed:  time.Now().Add(-10 * 24 * time.Hour),
	}
	eval.mu.Unlock()

	if eval.LearnedCount() != 2 {
		t.Fatalf("expected 2 entries, got %d", eval.LearnedCount())
	}

	eval.CleanupExpired()

	if eval.LearnedCount() != 1 {
		t.Errorf("after cleanup: expected 1 entry, got %d", eval.LearnedCount())
	}
}

func TestIsExternalDomain(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://localhost:8080", false},
		{"https://127.0.0.1/api", false},
		{"http://0.0.0.0:3000", false},
		{"http://192.168.1.1/data", false},
		{"http://10.0.0.1/api", false},
		{"https://172.16.0.1/v1", false},
		{"https://api.example.com", true},
		{"http://example.com", true},
		{"", false},
	}

	for _, tc := range cases {
		got := isExternalDomain(tc.url)
		if got != tc.want {
			t.Errorf("isExternalDomain(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}
