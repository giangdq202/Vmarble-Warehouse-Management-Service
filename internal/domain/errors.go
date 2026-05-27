package domain

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrAreaConservation   = errors.New("area conservation violated")
	ErrAlreadyFinalized   = errors.New("record already finalized")
	ErrPreconditionFailed = errors.New("precondition failed")
)

// BizError wraps a sentinel with a human-readable message. Details carries
// optional structured context that handlers can surface alongside the message
// (e.g. the list of pending loading_exception ids that block a seal).
type BizError struct {
	Sentinel error
	Message  string
	Details  map[string]any
}

func (e *BizError) Error() string { return e.Message }
func (e *BizError) Unwrap() error { return e.Sentinel }

func NewBizError(sentinel error, msg string) *BizError {
	return &BizError{Sentinel: sentinel, Message: msg}
}

// WithDetails attaches structured context to the error. Returns the same
// receiver so callers can chain at the return site.
func (e *BizError) WithDetails(details map[string]any) *BizError {
	e.Details = details
	return e
}
