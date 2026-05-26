package skills

import "strings"

// Skill 是加载后的技能实例。
type Skill struct {
	Manifest Manifest
	Content  string // prompt / system message 内容
	Trust    TrustLevel
	Source   Source
}

// Name 返回技能名称。
func (s *Skill) Name() string {
	return s.Manifest.Name
}

// IsTrusted 报告技能是否为完全信任。
func (s *Skill) IsTrusted() bool {
	return s.Trust == TrustTrusted
}

// MatchInput 检查用户输入是否触发该技能。
func (s *Skill) MatchInput(input string) bool {
	inputLower := strings.ToLower(input)

	// 排除关键词优先
	for _, kw := range s.Manifest.Activation.ExcludeKeywords {
		if strings.Contains(inputLower, strings.ToLower(kw)) {
			return false
		}
	}

	// 关键词匹配
	if len(s.Manifest.Activation.Keywords) > 0 {
		for _, kw := range s.Manifest.Activation.Keywords {
			if strings.Contains(inputLower, strings.ToLower(kw)) {
				return true
			}
		}
		return false
	}

	// 无激活条件时默认激活
	return true
}
