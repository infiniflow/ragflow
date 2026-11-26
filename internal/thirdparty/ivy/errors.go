package ivy

// Error is a sentinel error of ivy. Every format returns these errors, or
// wraps them with %w, and declares no sentinel error of its own (D11). A
// program tests for one with errors.Is.
type Error string

// Error satisfies the error interface.
func (err Error) Error() string {
	return string(err)
}

// The sentinel errors.
const (
	// ErrPassword is returned when a password is missing or wrong.
	ErrPassword Error = "password is missing or wrong"
	// ErrDRM is returned when a file holds DRM that ivy does not decode.
	ErrDRM Error = "file holds drm that ivy does not decode"
	// ErrNoPreview is returned when a file holds no preview.
	ErrNoPreview Error = "file holds no preview"
	// ErrPageRange is returned for a page past the last page.
	ErrPageRange Error = "page out of range"
	// ErrFormat is returned for a malformed file.
	ErrFormat Error = "malformed file"
	// ErrUnsupported is returned for a feature that ivy does not support.
	ErrUnsupported Error = "feature not supported"
	// ErrLimit is returned when a file exceeds a limit of its configuration.
	ErrLimit Error = "limit exceeded"
)

// PartialError reports a page that Render drew only in part. The pixmap holds
// the part. Err is the cause.
type PartialError struct {
	Err error
}

// Error satisfies the error interface.
func (err *PartialError) Error() string {
	return "partial render: " + err.Err.Error()
}

// Unwrap returns the cause.
func (err *PartialError) Unwrap() error {
	return err.Err
}
