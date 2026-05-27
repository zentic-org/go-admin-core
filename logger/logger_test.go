package logger

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestLogger(t *testing.T) {
	l := NewLogger(WithLevel(TraceLevel), WithName("test"))
	h1 := NewHelper(l).WithFields(map[string]interface{}{"key1": "val1"})
	h1.Trace("trace_msg1")
	h1.Warn("warn_msg1")

	h2 := NewHelper(l).WithFields(map[string]interface{}{"key2": "val2"})
	h2.Trace("trace_msg2")
	h2.Warn("warn_msg2")

	h3 := NewHelper(l).WithFields(map[string]interface{}{"key3": "val4"})
	h3.Info("test_msg")
	ctx := context.TODO()
	ctx = context.WithValue(ctx, &loggerKey{}, h3)
	v := ctx.Value(&loggerKey{})
	ll := v.(*Helper)
	ll.Info("test_msg")
}

func TestLogger2(t *testing.T) {
	slog.Info("hello", "count", 3, "name", "go-admin")
	slog.Info("hello", "count", 3)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("hello", "count", 3)

	logger2 := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger2.Info("hello", "count", 3)

	ctx := context.TODO()
	list := []any{"key1", "val1"}
	if len(list)%2 != 0 {
		slog.Error("")
	}
	slog.Info("key1", list...)
	//mp := map[string]interface{}{"key1": "val1"}

	//for s, i := range mp {
	//
	//}
	slog.InfoContext(ctx, "hello", list...)
	logger2.Info("hello", "count", 3)
	slog.InfoContext(ctx, "hello1", "count", 23)
}
