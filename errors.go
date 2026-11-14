package rootfsdiff

import (
	"errors"
	"fmt"
)

// Error categories support errors.Is through wrapping. I/O and cancellation
// errors retain their original identity through Error.Unwrap.
var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrUnsupportedInput = errors.New("unsupported input")
	ErrLimitExceeded    = errors.New("limit exceeded")
)

// ErrorKind identifies invalid or unsupported input. An empty kind leaves the
// underlying error's identity intact without assigning an input category.
type ErrorKind string

const (
	ErrorInvalidInput     ErrorKind = "invalid_input"
	ErrorUnsupportedInput ErrorKind = "unsupported_input"
)

// Error adds operation and archive-path context without losing an underlying
// I/O error. Path is an archive path, never an absolute host filename.
type Error struct {
	Op   string
	Path string
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string {
	s := e.Op
	if e.Path != "" {
		s += fmt.Sprintf(" %q", e.Path)
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	} else if e.Kind != "" {
		s += ": " + string(e.Kind)
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	return (e.Kind == ErrorInvalidInput && target == ErrInvalidInput) ||
		(e.Kind == ErrorUnsupportedInput && target == ErrUnsupportedInput)
}

// LimitError identifies the exhausted budget. Actual is the first observed
// value beyond Maximum, not necessarily the full size of an unread input.
// Err may retain an underlying read error when both facts are known.
type LimitError struct {
	Limit   string
	Maximum int64
	Actual  int64
	Err     error
}

func (e *LimitError) Error() string {
	s := fmt.Sprintf("%s limit exceeded: maximum %d, observed %d", e.Limit, e.Maximum, e.Actual)
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *LimitError) Unwrap() error        { return e.Err }
func (e *LimitError) Is(target error) bool { return target == ErrLimitExceeded }

func invalid(op, path, detail string) error {
	return &Error{Op: op, Path: path, Kind: ErrorInvalidInput, Err: errors.New(detail)}
}
