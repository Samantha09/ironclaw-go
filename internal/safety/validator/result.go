package validator

type ErrorCode string

const (
	ErrForbiddenContent  ErrorCode = "forbidden_content"
	ErrSuspiciousPattern ErrorCode = "suspicious_pattern"
	ErrTooLong           ErrorCode = "too_long"
	ErrInvalidEncoding   ErrorCode = "invalid_encoding"
)

type ValidationError struct {
	Field   string
	Message string
	Code    ErrorCode
}

type Result struct {
	IsValid  bool
	Errors   []ValidationError
	Warnings []string
}

func NewResult() Result {
	return Result{
		IsValid:  true,
		Errors:   []ValidationError{},
		Warnings: []string{},
	}
}

func (r Result) WithError(err ValidationError) Result {
	r.IsValid = false
	r.Errors = append(r.Errors, err)
	return r
}

func (r Result) WithWarning(warning string) Result {
	r.Warnings = append(r.Warnings, warning)
	return r
}

func (r Result) Merge(other Result) Result {
	if !other.IsValid {
		r.IsValid = false
	}
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
	return r
}
