package h2c

type wrapErr struct {
	msg string
	error
}

func (e wrapErr) Wrap(err error) error {
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
)
