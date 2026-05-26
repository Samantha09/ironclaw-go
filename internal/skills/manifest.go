package skills

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// TrustLevel 表示技能的信任级别。
type TrustLevel int

const (
	TrustInstalled TrustLevel = iota // 用户安装，受限信任
	TrustTrusted                     // 完全信任（如内置技能）
)

func (t TrustLevel) String() string {
	switch t {
	case TrustTrusted:
		return "trusted"
	case TrustInstalled:
		return "installed"
	default:
		return "unknown"
	}
}

// Source 表示技能的来源。
type Source int

const (
	SourceBundled Source = iota // 内置于二进制
	SourceUser                  // 用户安装
)

func (s Source) String() string {
	switch s {
	case SourceBundled:
		return "bundled"
	case SourceUser:
		return "user"
	default:
		return "unknown"
	}
}

// Manifest 是技能的 TOML frontmatter 定义。
type Manifest struct {
	Name        string           `toml:"name"`
	Version     string           `toml:"version"`
	Description string           `toml:"description"`
	Activation  Activation       `toml:"activation"`
	Credentials []CredentialSpec `toml:"credentials"`
}

// Activation 定义技能的自动激活条件。
type Activation struct {
	Keywords        []string `toml:"keywords"`
	ExcludeKeywords []string `toml:"exclude_keywords"`
	Tags            []string `toml:"tags"`
	Patterns        []string `toml:"patterns"`
}

// CredentialLocationType 表示凭证放置位置类型。
type CredentialLocationType string

const (
	LocationBearer    CredentialLocationType = "bearer"
	LocationBasicAuth CredentialLocationType = "basic_auth"
	LocationHeader    CredentialLocationType = "header"
	LocationQueryParam CredentialLocationType = "query_param"
)

// CredentialLocation 描述凭证如何附加到请求。
type CredentialLocation struct {
	Type     CredentialLocationType `toml:"type"`
	Username string                 `toml:"username,omitempty"`
	Name     string                 `toml:"name,omitempty"`     // header/query 名称
	Prefix   string                 `toml:"prefix,omitempty"`   // header 前缀
}

// CredentialSpec 定义技能需要的凭证。
type CredentialSpec struct {
	Name         string             `toml:"name"`
	Provider     string             `toml:"provider"`
	Location     CredentialLocation `toml:"location"`
	Hosts        []string           `toml:"hosts"`
	PathPatterns []string           `toml:"path_patterns"`
}

// Validate 检查清单字段有效性。
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("skill manifest: name is required")
	}
	if m.Version == "" {
		m.Version = "0.1.0"
	}
	for i, c := range m.Credentials {
		if c.Name == "" {
			return fmt.Errorf("skill manifest: credential %d: name is required", i)
		}
		if c.Provider == "" {
			return fmt.Errorf("skill manifest: credential %q: provider is required", c.Name)
		}
	}
	return nil
}

// ParseManifest 从 Markdown + TOML frontmatter 解析技能清单。
// 支持 +++ 或 --- 分隔的 frontmatter。
// 若无 frontmatter，返回 nil manifest 和完整 body。
func ParseManifest(content string) (*Manifest, string, error) {
	frontmatter, body, err := extractFrontmatter(content)
	if err != nil {
		return nil, "", fmt.Errorf("extract frontmatter: %w", err)
	}

	if frontmatter == "" {
		return nil, body, nil
	}

	var m Manifest
	if err := toml.Unmarshal([]byte(frontmatter), &m); err != nil {
		return nil, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, "", err
	}

	return &m, body, nil
}

// ParseManifestOnly 仅解析 frontmatter，不返回 body。
func ParseManifestOnly(content string) (*Manifest, error) {
	m, _, err := ParseManifest(content)
	return m, err
}

func extractFrontmatter(content string) (string, string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", fmt.Errorf("empty content")
	}

	// 尝试 +++ 分隔符
	if strings.HasPrefix(content, "+++") {
		end := strings.Index(content[3:], "+++")
		if end >= 0 {
			return strings.TrimSpace(content[3 : 3+end]), strings.TrimSpace(content[3+end+3:]), nil
		}
	}

	// 尝试 --- 分隔符
	if strings.HasPrefix(content, "---") {
		end := strings.Index(content[3:], "---")
		if end >= 0 {
			// 注意：--- 后面可能是 YAML，但我们尝试用 TOML 解析
			// 如果失败，调用方会收到解析错误
			return strings.TrimSpace(content[3 : 3+end]), strings.TrimSpace(content[3+end+3:]), nil
		}
	}

	// 没有 frontmatter：整个内容作为 body，返回空 manifest
	return "", content, nil
}
