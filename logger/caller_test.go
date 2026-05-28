package logger

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestLogrusCallerInfo 测试 Logrus Caller 信息是否正确
func TestLogrusCallerInfo(t *testing.T) {
	// 创建一个 buffer 来捕获日志输出
	buf := &bytes.Buffer{}

	// 创建 Logrus logger，输出到 buffer，使用 JSON 格式（便于解析字段）
	log := NewLogrusLogger(
		WithLevel(DebugLevel),
		WithOutput(buf),
		WithPath("test.log"), // 触发 JSON formatter
	)

	// 从业务代码调用日志
	log.Log(InfoLevel, "test message from caller_test.go") // 这一行是第 21 行

	output := buf.String()
	t.Logf("Log output: [%s]", output)
	t.Logf("Log output length: %d", len(output))

	// 验证：
	// 1. 应该有输出
	if len(output) == 0 {
		t.Error("Log output is empty!")
	}

	// 2. 应该包含 caller 字段
	if !strings.Contains(output, "caller") {
		t.Error("Log should contain caller field")
	}

	// 3. 应该包含 caller_test.go（实际调用位置）
	if !strings.Contains(output, "caller_test.go") {
		t.Error("Caller should show caller_test.go (actual caller)")
	}

	// 4. 不应该包含 logrus.go（logger 内部文件）
	if strings.Contains(output, "logrus.go") {
		t.Error("Caller should not show logrus.go (internal logger file)")
	}
}

// TestLogrusCallerWithLogf 测试 Logf 方法的 Caller 信息
func TestLogrusCallerWithLogf(t *testing.T) {
	buf := &bytes.Buffer{}
	log := NewLogrusLogger(
		WithLevel(DebugLevel),
		WithOutput(buf),
	)

	log.Logf(InfoLevel, "test message: %s", "logf caller") // 这一行是第 41 行

	output := buf.String()
	t.Logf("Log output: %s", output)

	if strings.Contains(output, "logrus.go") {
		t.Error("Logf caller should not show logrus.go")
	}

	if !strings.Contains(output, "caller_test.go") {
		t.Error("Logf caller should show caller_test.go")
	}
}

// helperFunction 辅助函数（模拟多层调用）
func helperFunction(log Logger) {
	log.Log(InfoLevel, "test from helper") // 这一行是第 54 行
}

// TestLogrusCallerWithHelper 测试多层调用的 Caller 信息
func TestLogrusCallerWithHelper(t *testing.T) {
	buf := &bytes.Buffer{}
	log := NewLogrusLogger(
		WithLevel(DebugLevel),
		WithOutput(buf),
	)

	helperFunction(log)

	output := buf.String()
	t.Logf("Log output: %s", output)

	// 应该显示 helperFunction 的位置（第 54 行），而不是 logrus.go
	if strings.Contains(output, "logrus.go") {
		t.Error("Helper caller should not show logrus.go")
	}

	if !strings.Contains(output, "caller_test.go") {
		t.Error("Helper caller should show caller_test.go")
	}
}

func TestLogrusStructuredCallerInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	log := NewLogrusLogger(
		WithLevel(DebugLevel),
		WithOutput(buf),
		WithPath(filepath.Join(t.TempDir(), "structured.log")),
	)

	adapter, ok := log.(*logrusAdapter)
	if !ok {
		t.Fatal("expected *logrusAdapter")
	}

	adapter.Info("structured message", FieldString("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "caller") {
		t.Error("structured log should contain caller field")
	}
	if !strings.Contains(output, "caller_test.go") {
		t.Error("structured log should show caller_test.go")
	}
}

func TestLogrusFieldsIsolation(t *testing.T) {
	buf := &bytes.Buffer{}
	baseFields := map[string]interface{}{"scope": "base"}
	log := NewLogrusLogger(
		WithLevel(DebugLevel),
		WithOutput(buf),
		WithPath(filepath.Join(t.TempDir(), "fields.log")),
		WithFields(baseFields),
	)

	baseFields["scope"] = "mutated"
	log.Log(InfoLevel, "base message")
	if strings.Contains(buf.String(), "mutated") {
		t.Error("logger should copy constructor fields")
	}

	opts := log.Options()
	opts.Fields["scope"] = "from-options"
	childFields := map[string]interface{}{"request_id": "req-1"}
	child := log.Fields(childFields)
	childFields["request_id"] = "req-2"
	child.Log(InfoLevel, "child message")

	output := buf.String()
	if strings.Contains(output, "from-options") {
		t.Error("mutating Options().Fields should not affect logger")
	}
	if strings.Contains(output, "req-2") {
		t.Error("mutating Fields() input should not affect logger")
	}
	if !strings.Contains(output, "req-1") {
		t.Error("child logger should retain original field values")
	}
}
