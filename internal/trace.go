package internal

import (
	"context"
	"net"
	"net/http/httptrace"
	"reflect"
)

var stdNetTraceKey, stdHttpTraceKey interface{}

type captureContext struct {
	context.Context
	capture func(reflect.Type)
}

func (c captureContext) Value(key interface{}) interface{} {
	c.capture(reflect.TypeOf(key))
	return nil
}

func init() {
	var stdNetTraceType, stdHttpTraceType reflect.Type

	capture := captureContext{context.Background(), nil}
	capture.capture = func(t reflect.Type) { stdNetTraceType = t }
	(&net.Dialer{}).DialContext(capture, "invalid", "")
	capture.capture = func(t reflect.Type) { stdHttpTraceType = t }
	httptrace.ContextClientTrace(capture)

	stdNetTraceKey = reflect.New(stdNetTraceType).Elem().Interface()
	stdHttpTraceKey = reflect.New(stdHttpTraceType).Elem().Interface()
}

func shadowStandardClientTrace(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, stdHttpTraceKey, nil)
	ctx = context.WithValue(ctx, stdNetTraceKey, nil)
	return ctx
}
