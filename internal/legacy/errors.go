package legacy

import "errors"

var (
	ErrAuth         = errors.New("legacy: authentication failed")
	ErrUncertainty  = errors.New("legacy: issue creation is uncertain")
	ErrAmbiguous    = errors.New("legacy: multiple matching issues")
	ErrValidation   = errors.New("legacy: validation failed")
	ErrUnsupported  = errors.New("legacy: command is not supported")
	ErrBusy         = errors.New("legacy: idempotent create is already running")
	ErrScanFailed   = errors.New("legacy: issue scan failed")
	ErrLostResponse = errors.New("legacy: form response was lost")
)

type codedError struct {
	kind error
	msg  string
}

func (e *codedError) Error() string {
	return e.msg
}

func (e *codedError) Unwrap() error {
	return e.kind
}

func wrap(kind error, msg string) error {
	return &codedError{kind: kind, msg: msg}
}
