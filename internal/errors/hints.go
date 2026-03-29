package errors

import "fmt"

// HintError wraps an error with a user-facing hint for resolution.
type HintError struct {
	Err  error
	Hint string
}

func (e *HintError) Error() string {
	return e.Err.Error()
}

func (e *HintError) Unwrap() error {
	return e.Err
}

// NewHintError creates a HintError wrapping err with the given hint.
func NewHintError(err error, hint string) *HintError {
	return &HintError{Err: err, Hint: hint}
}

// WithKVMHint wraps an error about /dev/kvm not being found.
func WithKVMHint(err error) *HintError {
	return NewHintError(err, "Hint: Is KVM enabled? Check: ls -la /dev/kvm")
}

// WithBridgeHint wraps an error about the bridge not being found.
func WithBridgeHint(err error) *HintError {
	return NewHintError(err, "Hint: Run 'fcm init' first")
}

// WithImageHint wraps an error about an image not being found.
func WithImageHint(err error) *HintError {
	return NewHintError(err, "Hint: Run 'fcm pull <image>' or 'fcm images --available'")
}

// WithQemuImgHint wraps an error about qemu-img not being found.
func WithQemuImgHint(err error) *HintError {
	return NewHintError(err, "Hint: Install with: sudo apt-get install qemu-utils")
}

// WithPermissionHint wraps a permission denied error.
func WithPermissionHint(err error) *HintError {
	return NewHintError(err, "Hint: fcm requires root. Run with sudo.")
}

// FormatError formats an error, including the hint if it is a HintError.
func FormatError(err error) string {
	if he, ok := err.(*HintError); ok {
		return fmt.Sprintf("Error: %v\n%s", he.Err, he.Hint)
	}
	return fmt.Sprintf("Error: %v", err)
}
