package extensions

import "fmt"

// ExtensionError 是扩展操作的错误类型。
type ExtensionError struct {
	Op  string
	Key string
	Err error
}

func (e *ExtensionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("extension %s %s: %v", e.Op, e.Key, e.Err)
	}
	return fmt.Sprintf("extension %s %s", e.Op, e.Key)
}

func (e *ExtensionError) Unwrap() error { return e.Err }

// Error constructors.

func ErrNotFound(name string) error {
	return &ExtensionError{Op: "not_found", Key: name, Err: fmt.Errorf("extension not found")}
}

func ErrAlreadyInstalled(name string) error {
	return &ExtensionError{Op: "already_installed", Key: name, Err: fmt.Errorf("extension already installed")}
}

func ErrNotInstalled(name string) error {
	return &ExtensionError{Op: "not_installed", Key: name, Err: fmt.Errorf("extension not installed")}
}

func ErrAuthFailed(name string, err error) error {
	return &ExtensionError{Op: "auth_failed", Key: name, Err: err}
}

func ErrActivationFailed(name string, err error) error {
	return &ExtensionError{Op: "activation_failed", Key: name, Err: err}
}

func ErrInstallFailed(name string, err error) error {
	return &ExtensionError{Op: "install_failed", Key: name, Err: err}
}

func ErrDiscoveryFailed(reason string) error {
	return &ExtensionError{Op: "discovery_failed", Key: reason, Err: fmt.Errorf("discovery failed")}
}

func ErrInvalidURL(url string) error {
	return &ExtensionError{Op: "invalid_url", Key: url, Err: fmt.Errorf("invalid URL")}
}

func ErrDownloadFailed(reason string) error {
	return &ExtensionError{Op: "download_failed", Key: reason, Err: fmt.Errorf("download failed")}
}

func ErrConfig(msg string) error {
	return &ExtensionError{Op: "config", Key: msg, Err: fmt.Errorf("config error")}
}
