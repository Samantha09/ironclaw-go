package extensions

import "fmt"

// ExtensionKind 表示扩展类型。
type ExtensionKind string

const (
	KindMcpServer    ExtensionKind = "mcp_server"
	KindWasmTool     ExtensionKind = "wasm_tool"
	KindWasmChannel  ExtensionKind = "wasm_channel"
	KindChannelRelay ExtensionKind = "channel_relay"
	KindAcpAgent     ExtensionKind = "acp_agent"
)

func (k ExtensionKind) String() string { return string(k) }

// ExtensionSource 表示扩展来源。
type ExtensionSource struct {
	Type string `json:"type"`

	// McpUrl
	URL string `json:"url,omitempty"`

	// WasmDownload
	WasmURL          string `json:"wasm_url,omitempty"`
	CapabilitiesURL  string `json:"capabilities_url,omitempty"`

	// WasmBuildable
	SourceDir string `json:"source_dir,omitempty"`
	BuildDir  string `json:"build_dir,omitempty"`
	CrateName string `json:"crate_name,omitempty"`

	// ChannelRelay
	RelayURL string `json:"relay_url,omitempty"`
}

// AuthHint 表示认证方式提示。
type AuthHint struct {
	Type string `json:"type"`

	// OAuthPreConfigured
	SetupURL string `json:"setup_url,omitempty"`
}

const (
	AuthHintDcr                = "dcr"
	AuthHintOAuthPreConfigured = "oauth_pre_configured"
	AuthHintCapabilitiesAuth   = "capabilities_auth"
	AuthHintNone               = "none"
	AuthHintChannelRelayOAuth  = "channel_relay_oauth"
)

// RegistryEntry 是注册表中的扩展条目。
type RegistryEntry struct {
	Name           string            `json:"name"`
	DisplayName    string            `json:"display_name"`
	Kind           ExtensionKind     `json:"kind"`
	Description    string            `json:"description"`
	Keywords       []string          `json:"keywords,omitempty"`
	Source         ExtensionSource   `json:"source"`
	FallbackSource *ExtensionSource  `json:"fallback_source,omitempty"`
	AuthHint       AuthHint          `json:"auth_hint"`
	Version        string            `json:"version,omitempty"`
	Hidden         bool              `json:"hidden,omitempty"`
}

// ResultSource 表示搜索结果来源。
type ResultSource string

const (
	ResultSourceRegistry   ResultSource = "registry"
	ResultSourceDiscovered ResultSource = "discovered"
)

// SearchResult 是扩展搜索结果。
type SearchResult struct {
	RegistryEntry
	Source    ResultSource `json:"source"`
	Validated bool         `json:"validated,omitempty"`
}

// InstallResult 是安装结果。
type InstallResult struct {
	Name    string        `json:"name"`
	Kind    ExtensionKind `json:"kind"`
	Message string        `json:"message"`
}

// UpgradeResult 是升级结果。
type UpgradeResult struct {
	Results []UpgradeOutcome `json:"results"`
	Message string           `json:"message"`
}

// UpgradeOutcome 是单个扩展的升级结果。
type UpgradeOutcome struct {
	Name   string        `json:"name"`
	Kind   ExtensionKind `json:"kind"`
	Status string        `json:"status"`
	Detail string        `json:"detail"`
}

// ToolAuthState 是工具认证状态。
type ToolAuthState string

const (
	ToolAuthReady      ToolAuthState = "ready"
	ToolAuthNeedsAuth  ToolAuthState = "needs_auth"
	ToolAuthNeedsSetup ToolAuthState = "needs_setup"
	ToolAuthNoAuth     ToolAuthState = "no_auth"
)

// AuthStatus 是认证状态的类型化表示。
type AuthStatus string

const (
	AuthStatusAuthenticated         AuthStatus = "authenticated"
	AuthStatusNoAuthRequired        AuthStatus = "no_auth_required"
	AuthStatusAwaitingAuthorization AuthStatus = "awaiting_authorization"
	AuthStatusAwaitingToken         AuthStatus = "awaiting_token"
	AuthStatusNeedsSetup            AuthStatus = "needs_setup"
)

// AuthResult 是认证结果。
type AuthResult struct {
	Name         string        `json:"name"`
	Kind         ExtensionKind `json:"kind"`
	Status       AuthStatus    `json:"status"`
	AuthURL      string        `json:"auth_url,omitempty"`
	CallbackType string        `json:"callback_type,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
	SetupURL     string        `json:"setup_url,omitempty"`
	AwaitingToken bool         `json:"awaiting_token,omitempty"`
}

// IsAuthenticated 报告是否已完成认证。
func (a AuthResult) IsAuthenticated() bool {
	return a.Status == AuthStatusAuthenticated
}

// IsAwaitingToken 报告是否等待用户输入 token。
func (a AuthResult) IsAwaitingToken() bool {
	return a.Status == AuthStatusAwaitingToken
}

// ActivateResult 是激活结果。
type ActivateResult struct {
	Name         string        `json:"name"`
	Kind         ExtensionKind `json:"kind"`
	ToolsLoaded  []string      `json:"tools_loaded"`
	Message      string        `json:"message"`
}

// ExtensionPhase 是扩展生命周期阶段。
type ExtensionPhase string

const (
	PhaseInstalled        ExtensionPhase = "installed"
	PhaseNeedsSetup       ExtensionPhase = "needs_setup"
	PhaseNeedsAuth        ExtensionPhase = "needs_auth"
	PhaseNeedsActivation  ExtensionPhase = "needs_activation"
	PhaseActivating       ExtensionPhase = "activating"
	PhaseReady            ExtensionPhase = "ready"
	PhaseError            ExtensionPhase = "error"
)

// EnsureReadyIntent 是检查就绪的意图。
type EnsureReadyIntent string

const (
	IntentUseCapability   EnsureReadyIntent = "use_capability"
	IntentPostInstall     EnsureReadyIntent = "post_install"
	IntentExplicitAuth    EnsureReadyIntent = "explicit_auth"
	IntentExplicitActivate EnsureReadyIntent = "explicit_activate"
)

// EnsureReadyOutcome 是就绪检查的结果。
type EnsureReadyOutcome struct {
	OutcomeType string // "ready", "needs_auth", "needs_setup"

	Name       string
	Kind       ExtensionKind
	Phase      ExtensionPhase
	Auth       *AuthResult
	Activation *ActivateResult
	Instructions string
	SetupURL   string
	CredentialName string
}

// Ready 构造 Ready 结果。
func ReadyOutcome(name string, kind ExtensionKind, phase ExtensionPhase, activation *ActivateResult) EnsureReadyOutcome {
	return EnsureReadyOutcome{
		OutcomeType: "ready",
		Name:        name,
		Kind:        kind,
		Phase:       phase,
		Activation:  activation,
	}
}

// NeedsAuth 构造 NeedsAuth 结果。
func NeedsAuthOutcome(name string, kind ExtensionKind, phase ExtensionPhase, auth AuthResult, credentialName string) EnsureReadyOutcome {
	return EnsureReadyOutcome{
		OutcomeType:    "needs_auth",
		Name:           name,
		Kind:           kind,
		Phase:          phase,
		Auth:           &auth,
		CredentialName: credentialName,
	}
}

// NeedsSetup 构造 NeedsSetup 结果。
func NeedsSetupOutcome(name string, kind ExtensionKind, phase ExtensionPhase, instructions, setupURL string) EnsureReadyOutcome {
	return EnsureReadyOutcome{
		OutcomeType:  "needs_setup",
		Name:         name,
		Kind:         kind,
		Phase:        phase,
		Instructions: instructions,
		SetupURL:     setupURL,
	}
}

// Phase 返回结果对应的生命周期阶段。
func (e EnsureReadyOutcome) PhaseValue() ExtensionPhase {
	return e.Phase
}

// InteractiveLoginStartResult 是交互式登录启动结果。
type InteractiveLoginStartResult struct {
	SessionID    string `json:"session_id"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	QRCodeURL    string `json:"qr_code_url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// InteractiveLoginPollResult 是交互式登录轮询结果。
type InteractiveLoginPollResult struct {
	SessionID  string `json:"session_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	QRCodeURL  string `json:"qr_code_url,omitempty"`
	Activated  *bool  `json:"activated,omitempty"`
}

// InstalledExtension 表示已安装的扩展。
type InstalledExtension struct {
	Name             string        `json:"name"`
	Kind             ExtensionKind `json:"kind"`
	DisplayName      string        `json:"display_name,omitempty"`
	Description      string        `json:"description,omitempty"`
	URL              string        `json:"url,omitempty"`
	Authenticated    bool          `json:"authenticated"`
	Active           bool          `json:"active"`
	Tools            []string      `json:"tools,omitempty"`
	NeedsSetup       bool          `json:"needs_setup,omitempty"`
	HasAuth          bool          `json:"has_auth,omitempty"`
	RequiresBinding  bool          `json:"requires_binding,omitempty"`
	Installed        bool          `json:"installed,omitempty"`
	ActivationError  string        `json:"activation_error,omitempty"`
	Version          string        `json:"version,omitempty"`
}

// LatentProviderAction 是潜在提供者操作。
type LatentProviderAction struct {
	ActionName       string `json:"action_name"`
	ProviderExtension string `json:"provider_extension"`
	Description      string `json:"description"`
	ParametersSchema map[string]any `json:"parameters_schema"`
}

// ConfigureResult 是配置结果。
type ConfigureResult struct {
	Message         string `json:"message"`
	Activated       bool   `json:"activated"`
	PairingRequired bool   `json:"pairing_required"`
	AuthURL         string `json:"auth_url,omitempty"`
}

func defaultTrue() bool { return true }

// NewInstalledExtension 创建带默认值的 InstalledExtension。
func NewInstalledExtension(name string, kind ExtensionKind) InstalledExtension {
	return InstalledExtension{
		Name:      name,
		Kind:      kind,
		Installed: true,
	}
}

// String implements fmt.Stringer for ExtensionKind.
func (k ExtensionKind) GoString() string { return string(k) }

// Validate 检查 RegistryEntry 的有效性。
func (r *RegistryEntry) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("registry entry: name is required")
	}
	if r.Kind == "" {
		return fmt.Errorf("registry entry %q: kind is required", r.Name)
	}
	return nil
}
