package errors

type wrapErr struct {
	msg string
	error
}

func (e wrapErr) Wrap(err error) error {
	if err == nil {
		return e
	}
	return wrapErr{e.msg, err}
}

func (e wrapErr) Unwrap() error {
	return e.error
}

func (e wrapErr) Is(err error) bool {
	if _, ok := err.(wrapErr); ok {
		return true
	}
	return false
}

var (
	ErrStreamCancelled = wrapErr{"request cancelled", nil}
	ErrStreamReset     = wrapErr{"stream already been reset", nil}
)
