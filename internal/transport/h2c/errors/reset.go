package errors

type StreamReset struct {
	reason string
}

func (s StreamReset) Error() string {
	return s.reason
}

var (
	ErrStreamResetRemote = StreamReset{reason: "remote stream reset"}
)
