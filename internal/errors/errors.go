package errors

import "fmt"

// ConfigError — missing or invalid configuration.
type ConfigError struct {
	Key  string
	Hint string
}

func NewConfigError(key, hint string) *ConfigError {
	return &ConfigError{Key: key, Hint: hint}
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s — %s", e.Key, e.Hint)
}

// ToolError — tool execution failure.
type ToolError struct {
	Tool    string
	Message string
	Code    string
}

func NewToolErrorExecutionFailed(tool, msg string) *ToolError {
	return &ToolError{Tool: tool, Message: msg, Code: "execution_failed"}
}

func (e *ToolError) Error() string {
	return fmt.Sprintf("tool '%s' failed (%s): %s", e.Tool, e.Code, e.Message)
}

// ChannelError — channel communication failure.
type ChannelError struct {
	Channel string
	Op      string
	Cause   error
}

func (e *ChannelError) Error() string {
	return fmt.Sprintf("channel '%s' %s failed: %v", e.Channel, e.Op, e.Cause)
}

// DatabaseError — persistence failure.
type DatabaseError struct {
	Op  string
	Err error
}

func (e *DatabaseError) Error() string {
	return fmt.Sprintf("database %s: %v", e.Op, e.Err)
}
