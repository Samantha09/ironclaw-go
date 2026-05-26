package registry

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/nearai/ironclaw-go/internal/extensions"
)

// ManifestKind 是扩展清单中声明的类型。
type ManifestKind string

const (
	KindTool     ManifestKind = "tool"
	KindChannel  ManifestKind = "channel"
	KindMcpServer ManifestKind = "mcp_server"
)

func (k ManifestKind) String() string { return string(k) }

func (k ManifestKind) ToExtensionKind() extensions.ExtensionKind {
	switch k {
	case KindTool:
		return extensions.KindWasmTool
	case KindChannel:
		return extensions.KindWasmChannel
	case KindMcpServer:
		return extensions.KindMcpServer
	default:
		return extensions.KindWasmTool
	}
}

// ExtensionManifest 描述单个扩展的清单。
type ExtensionManifest struct {
	Name        string                  `json:"name"`
	DisplayName string                  `json:"display_name"`
	Kind        ManifestKind            `json:"kind"`
	Version     string                  `json:"version,omitempty"`
	Description string                  `json:"description"`
	Keywords    []string                `json:"keywords,omitempty"`
	Source      *SourceSpec             `json:"source,omitempty"`
	Artifacts   map[string]ArtifactSpec `json:"artifacts,omitempty"`
	AuthSummary *AuthSummary            `json:"auth_summary,omitempty"`
	Tags        []string                `json:"tags,omitempty"`
	URL         string                  `json:"url,omitempty"`  // MCP server only
	Auth        string                  `json:"auth,omitempty"` // MCP server only
	Hidden      bool                    `json:"hidden,omitempty"`
}

// SourceSpec 是源码位置。
type SourceSpec struct {
	Dir          string `json:"dir"`
	Capabilities string `json:"capabilities"`
	CrateName    string `json:"crate_name"`
}

// ArtifactSpec 是预构建产物。
type ArtifactSpec struct {
	URL             string `json:"url,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	CapabilitiesURL string `json:"capabilities_url,omitempty"`
}

// AuthSummary 是认证需求摘要。
type AuthSummary struct {
	Method    string   `json:"method,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	Secrets   []string `json:"secrets,omitempty"`
	SharedAuth string  `json:"shared_auth,omitempty"`
	SetupURL  string   `json:"setup_url,omitempty"`
}

// BundleDefinition 是 Bundle 定义。
type BundleDefinition struct {
	DisplayName string   `json:"display_name"`
	Description string   `json:"description,omitempty"`
	Extensions  []string `json:"extensions"`
	SharedAuth  string   `json:"shared_auth,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// BundlesFile 是 `_bundles.json` 顶层结构。
type BundlesFile struct {
	Bundles map[string]BundleDefinition `json:"bundles"`
}

// Validate 检查清单字段有效性。
func (m *ExtensionManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest name is required")
	}
	if m.Kind == "" {
		return fmt.Errorf("manifest %q: kind is required", m.Name)
	}
	if m.DisplayName == "" {
		return fmt.Errorf("manifest %q: display_name is required", m.Name)
	}
	return nil
}

// ToRegistryEntry 将清单转换为 RegistryEntry。
func (m *ExtensionManifest) ToRegistryEntry() (*extensions.RegistryEntry, error) {
	if m.Kind == KindMcpServer {
		return m.toMcpRegistryEntry()
	}
	return m.toWasmRegistryEntry(), nil
}

func (m *ExtensionManifest) toMcpRegistryEntry() (*extensions.RegistryEntry, error) {
	if m.URL == "" {
		return nil, fmt.Errorf("MCP server manifest %q missing url", m.Name)
	}

	var hint extensions.AuthHint
	switch {
	case m.Auth == "" || m.Auth == "dcr":
		hint = extensions.AuthHint{Type: extensions.AuthHintDcr}
	case m.Auth == "none":
		hint = extensions.AuthHint{Type: extensions.AuthHintNone}
	case strings.HasPrefix(m.Auth, "oauth_pre_configured:"):
		hint = extensions.AuthHint{
			Type:     extensions.AuthHintOAuthPreConfigured,
			SetupURL: strings.TrimPrefix(m.Auth, "oauth_pre_configured:"),
		}
	default:
		hint = extensions.AuthHint{Type: extensions.AuthHintDcr}
	}

	return &extensions.RegistryEntry{
		Name:        m.Name,
		DisplayName: m.DisplayName,
		Kind:        extensions.KindMcpServer,
		Description: m.Description,
		Keywords:    m.Keywords,
		Source:      extensions.ExtensionSource{Type: "mcp_url", URL: m.URL},
		AuthHint:    hint,
		Version:     m.Version,
		Hidden:      m.Hidden,
	}, nil
}

func (m *ExtensionManifest) toWasmRegistryEntry() *extensions.RegistryEntry {
	var source extensions.ExtensionSource
	var fallback *extensions.ExtensionSource

	buildable := m.buildableSource()

	// 优先使用预构建产物下载
	if artifact, ok := m.Artifacts["wasm32-wasip2"]; ok && artifact.URL != "" {
		source = extensions.ExtensionSource{
			Type:            "wasm_download",
			WasmURL:         artifact.URL,
			CapabilitiesURL: artifact.CapabilitiesURL,
		}
		if buildable != nil {
			fb := *buildable
			fallback = &fb
		}
	} else if buildable != nil {
		source = *buildable
	} else {
		source = extensions.ExtensionSource{Type: "wasm_buildable"}
	}

	var hint extensions.AuthHint
	if m.AuthSummary != nil {
		switch m.AuthSummary.Method {
		case "oauth", "manual":
			hint = extensions.AuthHint{Type: extensions.AuthHintCapabilitiesAuth}
		case "none":
			hint = extensions.AuthHint{Type: extensions.AuthHintNone}
		default:
			hint = extensions.AuthHint{Type: extensions.AuthHintCapabilitiesAuth}
		}
	} else {
		hint = extensions.AuthHint{Type: extensions.AuthHintNone}
	}

	return &extensions.RegistryEntry{
		Name:           m.Name,
		DisplayName:    m.DisplayName,
		Kind:           m.Kind.ToExtensionKind(),
		Description:    m.Description,
		Keywords:       m.Keywords,
		Source:         source,
		FallbackSource: fallback,
		AuthHint:       hint,
		Version:        m.Version,
		Hidden:         m.Hidden,
	}
}

func (m *ExtensionManifest) buildableSource() *extensions.ExtensionSource {
	if m.Source == nil {
		return nil
	}
	return &extensions.ExtensionSource{
		Type:      "wasm_buildable",
		SourceDir: m.Source.Dir,
		BuildDir:  m.Source.Dir,
		CrateName: m.Source.CrateName,
	}
}

// LoadManifest 从 JSON 文件加载 ExtensionManifest。
func LoadManifest(path string) (*ExtensionManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", path, err)
	}
	var m ExtensionManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %q: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validate manifest %q: %w", path, err)
	}
	return &m, nil
}

// LoadBundles 从 JSON 文件加载 BundlesFile。
func LoadBundles(path string) (*BundlesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundles %q: %w", path, err)
	}
	var b BundlesFile
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse bundles %q: %w", path, err)
	}
	if b.Bundles == nil {
		b.Bundles = make(map[string]BundleDefinition)
	}
	return &b, nil
}

// MergeBundles 将多个 BundlesFile 合并为一个。
func MergeBundles(files ...*BundlesFile) *BundlesFile {
	merged := &BundlesFile{Bundles: make(map[string]BundleDefinition)}
	for _, f := range files {
		if f == nil {
			continue
		}
		maps.Copy(merged.Bundles, f.Bundles)
	}
	return merged
}

// DiscoverManifests 扫描目录下的所有 .json manifest 文件。
func DiscoverManifests(dir string) ([]*ExtensionManifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	var manifests []*ExtensionManifest
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		m, err := LoadManifest(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue // 跳过无效 manifest
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}
