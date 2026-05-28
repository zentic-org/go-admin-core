package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// loggerSourceDir go-admin-core/logger 包的源码目录（参考 GORM 的 gormSourceDir）
	// 用于在跳过调用栈时准确识别 logger 内部文件
	// 采用 HasPrefix 匹配，完美支持 -trimpath 编译
	loggerSourceDir string
)

func init() {
	// 获取当前文件（logrus.go）所在目录的绝对路径
	// 此操作在编译时执行，不受 -trimpath 影响
	_, file, _, _ := runtime.Caller(0)
	// 使用 sourceDir 函数计算包根目录（与 GORM 一致）
	loggerSourceDir = sourceDir(file)
}

// sourceDir 计算源码目录（参考 GORM 的实现）
// 输入：/path/to/go-admin-core/logger/logrus.go
// 输出：/path/to/go-admin-core/logger/
func sourceDir(file string) string {
	dir := filepath.Dir(file) // /path/to/go-admin-core/logger
	// 直接返回 logger 包目录，统一使用 / 并加上结尾斜杠
	return filepath.ToSlash(dir) + "/"
}

// addDateToFilename 在文件名中添加日期（保留扩展名）
// 例如：/var/log/app.log -> /var/log/app-2026-05-28.log
//
//	/var/log/access.log -> /var/log/access-2026-05-28.log
func addDateToFilename(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// 分离文件名和扩展名
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	// 添加日期：YYYY-MM-DD
	dateStr := time.Now().Format("2006-01-02")
	newBase := name + "-" + dateStr + ext

	return filepath.Join(dir, newBase)
}

func cloneLogrusOptions(opts Options) Options {
	cloned := opts
	if opts.Fields != nil {
		cloned.Fields = copyFields(opts.Fields)
	}
	return cloned
}

func mergeFieldMaps(base, extra map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}

	merged := make(map[string]interface{}, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

// logrusAdapter Logrus 适配器（生态丰富，插件多）
type logrusAdapter struct {
	logger *logrus.Logger
	entry  *logrus.Entry
	opts   Options
}

// NewLogrusLogger 创建基于 Logrus 的 logger
func NewLogrusLogger(opts ...Option) Logger {
	options := DefaultOptions()
	options.Level = InfoLevel
	options.CallerSkipCount = 2
	// 注意：不设置 options.Name 默认值，允许调用者通过 WithName() 传入

	for _, o := range opts {
		o(&options)
	}

	options = cloneLogrusOptions(options)

	baseLogger := newLogrusAdapter(options)
	return wrapLogrusLogger(baseLogger, options)
}

func newLogrusAdapter(options Options) *logrusAdapter {
	// 创建 Logrus Logger
	log := logrus.New()

	// 设置日志级别
	log.SetLevel(toLogrusLevel(options.Level))

	// 设置输出
	writers := []io.Writer{}

	// 自定义输出（用于测试或特殊场景）
	if options.Out != nil {
		writers = append(writers, options.Out)
	}

	// 控制台输出
	if options.Stdout && options.Out == nil {
		writers = append(writers, os.Stdout)
	}

	// 文件输出（支持日志轮转）
	if options.Path != "" {
		// 确保目录存在
		dir := filepath.Dir(options.Path)
		if err := os.MkdirAll(dir, 0755); err == nil {
			// 在文件名中添加日期，让 lumberjack 自动处理轮转和压缩
			// 例如：/var/log/app.log -> /var/log/app-2026-05-28.log
			pathWithDate := addDateToFilename(options.Path)

			fileWriter := &lumberjack.Logger{
				Filename:   pathWithDate,       // 使用带日期的路径
				MaxSize:    options.MaxSize,    // MB
				MaxBackups: options.MaxBackups, // 最多保留文件数
				MaxAge:     options.MaxAge,     // 天
				Compress:   options.Compress,   // 启用压缩
				LocalTime:  options.LocalTime,  // 使用本地时间
			}
			writers = append(writers, fileWriter)
		} else {
			fmt.Fprintf(os.Stderr, "logger: create log directory %s: %v\n", dir, err)
		}
	}

	if len(writers) > 0 {
		log.SetOutput(io.MultiWriter(writers...))
	} else {
		// 默认输出到 os.Stderr
		log.SetOutput(os.Stderr)
	}

	// 设置格式化器
	if options.Path != "" && !options.Stdout {
		// 仅文件输出使用 JSON 格式
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "time",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "msg",
				logrus.FieldKeyFunc:  "caller",
			},
		})
	} else {
		// 控制台输出使用自定义简洁格式
		log.SetFormatter(&ConsoleFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			ForceColors:     true,
		})
	}

	// 禁用 Logrus 内置的 ReportCaller（我们在 getEntryWithCaller 中手动处理）
	log.SetReportCaller(false)

	// 创建 Entry（带默认字段）
	entry := logrus.NewEntry(log)
	if options.Name != "" {
		entry = entry.WithField("logger", options.Name)
	}
	if len(options.Fields) > 0 {
		entry = entry.WithFields(logrus.Fields(options.Fields))
	}

	return &logrusAdapter{
		logger: log,
		entry:  entry,
		opts:   options,
	}
}

func wrapLogrusLogger(baseLogger Logger, options Options) Logger {
	// 应用高级功能（采样、异步、脱敏）
	var finalLogger Logger = baseLogger

	// 1. 脱敏（最内层）- 在字段传入时立即脱敏
	if options.Context != nil {
		if config, ok := options.Context.Value("sanitizer").(SanitizerConfig); ok && config.Enabled {
			finalLogger = NewSanitizerLogger(finalLogger, config)
		}
	}

	// 2. 采样（中间层）- 在脱敏后决定是否记录
	if options.Context != nil {
		if config, ok := options.Context.Value("sampling").(SamplingConfig); ok && config.Tick > 0 {
			finalLogger = NewSamplingLogger(finalLogger, config)
		}
	}

	// 3. 异步（最外层）- 在采样决策后异步写入
	if options.Context != nil {
		if config, ok := options.Context.Value("async").(AsyncConfig); ok && config.BufferSize > 0 {
			finalLogger = NewAsyncLogger(finalLogger, config)
		}
	}

	return finalLogger
}

func (l *logrusAdapter) Init(opts ...Option) error {
	for _, o := range opts {
		o(&l.opts)
	}

	l.opts = cloneLogrusOptions(l.opts)
	rebuilt := newLogrusAdapter(l.opts)
	l.logger = rebuilt.logger
	l.entry = rebuilt.entry
	l.opts = rebuilt.opts
	return nil
}

func (l *logrusAdapter) Options() Options {
	return cloneLogrusOptions(l.opts)
}

func (l *logrusAdapter) Fields(fields map[string]interface{}) Logger {
	mergedFields := mergeFieldMaps(l.opts.Fields, copyFields(fields))
	childOpts := cloneLogrusOptions(l.opts)
	childOpts.Fields = mergedFields

	return &logrusAdapter{
		logger: l.logger,
		entry:  l.entry.WithFields(logrus.Fields(copyFields(fields))),
		opts:   childOpts,
	}
}

func (l *logrusAdapter) Log(level Level, v ...interface{}) {
	l.writeArgs(level, 1, v...)
}

func (l *logrusAdapter) Logf(level Level, format string, v ...interface{}) {
	l.writeFormat(level, 1, format, v...)
}

func (l *logrusAdapter) writeArgs(level Level, callerSkip int, v ...interface{}) {
	entry := l.getEntryWithCaller(callerSkip)
	switch level {
	case TraceLevel:
		entry.Trace(v...)
	case DebugLevel:
		entry.Debug(v...)
	case InfoLevel:
		entry.Info(v...)
	case WarnLevel:
		entry.Warn(v...)
	case ErrorLevel:
		entry.Error(v...)
	case FatalLevel:
		entry.Fatal(v...)
	}
}

func (l *logrusAdapter) writeFormat(level Level, callerSkip int, format string, v ...interface{}) {
	entry := l.getEntryWithCaller(callerSkip)
	switch level {
	case TraceLevel:
		entry.Tracef(format, v...)
	case DebugLevel:
		entry.Debugf(format, v...)
	case InfoLevel:
		entry.Infof(format, v...)
	case WarnLevel:
		entry.Warnf(format, v...)
	case ErrorLevel:
		entry.Errorf(format, v...)
	case FatalLevel:
		entry.Fatalf(format, v...)
	}
}

// shouldSkipFrame 判断是否应该跳过该调用栈帧（参考 GORM 的跳过逻辑）
// 返回 true = 跳过（logger 包或第三方库），false = 业务代码
// 判断逻辑：
// 1. logger 包内部文件：使用 HasPrefix（不受 -trimpath 影响）
// 2. 第三方库：检查 /go/pkg/mod/（业务代码使用 -trimpath 后是相对路径）
// 3. 标准库：runtime、testing 等
func shouldSkipFrame(file string) bool {
	// 0. 测试文件不跳过（即使在 logger 包内）
	if strings.HasSuffix(file, "_test.go") {
		return false
	}

	// 1. logger 包内部文件（使用 HasPrefix，不受 -trimpath 影响）
	if strings.HasPrefix(file, loggerSourceDir) {
		return true
	}

	// 2. 第三方库：检查是否在 go/pkg/mod 目录下
	//    业务代码使用 -trimpath 编译后是相对路径，不会包含此路径
	if strings.Contains(file, "/go/pkg/mod/") {
		return true
	}

	// 3. runtime 和 testing 包（Go 标准库）
	if strings.Contains(file, "/runtime/") || strings.Contains(file, "/testing/") {
		return true
	}

	// 其他都是业务代码
	return false
}

// getEntryWithCaller 获取带有正确 Caller 信息的 Entry（完全参考 GORM 的 FileWithLineNum 实现）
// 性能优化：
// - 使用固定数组 [13]uintptr 避免堆分配
// - 从第 3 层开始（跳过 runtime.Callers、getEntryWithCaller、Log/Logf）
// - 找到第一个业务代码即停止遍历
func (l *logrusAdapter) getEntryWithCaller(skip int) *logrus.Entry {
	pcs := [13]uintptr{} // 固定数组，栈分配，高性能
	callersSkip := 1 + l.opts.CallerSkipCount + skip
	length := runtime.Callers(callersSkip, pcs[:])
	frames := runtime.CallersFrames(pcs[:length])

	for i := 0; i < length; i++ {
		frame, _ := frames.Next()

		// 调试：打印调用栈（仅在开发环境）
		if os.Getenv("DEBUG_CALLER") == "1" {
			skip := shouldSkipFrame(frame.File)
			fmt.Fprintf(os.Stderr, "[CALLER] %s:%d | skip=%v\n", frame.File, frame.Line, skip)
		}

		// 找到第一个业务代码（参考 GORM：!HasPrefix && !Contains）
		if !shouldSkipFrame(frame.File) {
			if os.Getenv("DEBUG_CALLER") == "1" {
				fmt.Fprintf(os.Stderr, "[CALLER] ✅ Found: %s:%d\n", frame.File, frame.Line)
			}
			return l.entry.WithField("caller", formatCaller(frame))
		}
	}

	if os.Getenv("DEBUG_CALLER") == "1" {
		fmt.Fprintf(os.Stderr, "[CALLER] ❌ No business code found\n")
	}
	return l.entry
}

// formatCaller 格式化调用者信息（保留绝对路径，与 GORM 一致）
func formatCaller(frame runtime.Frame) string {
	// 直接返回绝对路径 + 行号，与 GORM 的 utils.FileWithLineNum() 格式一致
	return fmt.Sprintf("%s:%d", frame.File, frame.Line)
}

func (l *logrusAdapter) String() string {
	return "logrus"
}

// 结构化日志 API（扩展接口）
func (l *logrusAdapter) Info(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(InfoLevel) {
		return
	}
	l.writeMessage(InfoLevel, 1, msg, fields...)
}

func (l *logrusAdapter) Debug(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(DebugLevel) {
		return
	}
	l.writeMessage(DebugLevel, 1, msg, fields...)
}

func (l *logrusAdapter) Warn(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(WarnLevel) {
		return
	}
	l.writeMessage(WarnLevel, 1, msg, fields...)
}

func (l *logrusAdapter) Error(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(ErrorLevel) {
		return
	}
	l.writeMessage(ErrorLevel, 1, msg, fields...)
}

func (l *logrusAdapter) writeMessage(level Level, callerSkip int, msg string, fields ...Field) {
	entry := l.getEntryWithCaller(callerSkip)
	if len(fields) > 0 {
		entry = entry.WithFields(toLogrusFields(fields))
	}

	switch level {
	case TraceLevel:
		entry.Trace(msg)
	case DebugLevel:
		entry.Debug(msg)
	case InfoLevel:
		entry.Info(msg)
	case WarnLevel:
		entry.Warn(msg)
	case ErrorLevel:
		entry.Error(msg)
	case FatalLevel:
		entry.Fatal(msg)
	}
}

func (l *logrusAdapter) WithContext(ctx context.Context) Logger {
	// 提取 context 中的字段
	fields := extractContextFields(ctx)
	if len(fields) == 0 {
		return l
	}
	return l.Fields(fields)
}

func (l *logrusAdapter) With(fields ...Field) Logger {
	fieldMap := fieldsToMap(fields)
	mergedFields := mergeFieldMaps(l.opts.Fields, fieldMap)
	childOpts := cloneLogrusOptions(l.opts)
	childOpts.Fields = mergedFields

	return &logrusAdapter{
		logger: l.logger,
		entry:  l.entry.WithFields(toLogrusFields(fields)),
		opts:   childOpts,
	}
}

func (l *logrusAdapter) Sync() error {
	// Logrus 不需要显式 Sync
	return nil
}

// 辅助函数

func toLogrusLevel(level Level) logrus.Level {
	switch level {
	case TraceLevel:
		return logrus.TraceLevel
	case DebugLevel:
		return logrus.DebugLevel
	case InfoLevel:
		return logrus.InfoLevel
	case WarnLevel:
		return logrus.WarnLevel
	case ErrorLevel:
		return logrus.ErrorLevel
	case FatalLevel:
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}

func toLogrusFields(fields []Field) logrus.Fields {
	return logrus.Fields(fieldsToMap(fields))
}

// Logrus Hook 支持

// AddHook 添加 Logrus Hook（Logrus 生态优势）
func (l *logrusAdapter) AddHook(hook logrus.Hook) {
	l.logger.AddHook(hook)
}

// GetLogrusLogger 获取底层 Logrus Logger（用于高级定制）
func (l *logrusAdapter) GetLogrusLogger() *logrus.Logger {
	return l.logger
}

// SetFormatter 设置格式化器
func (l *logrusAdapter) SetFormatter(formatter logrus.Formatter) {
	l.logger.SetFormatter(formatter)
}

// 常用 Hook 示例

// SentryHook Sentry 错误上报 Hook
type SentryHook struct {
	levels []logrus.Level
}

func NewSentryHook() *SentryHook {
	return &SentryHook{
		levels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
		},
	}
}

func (h *SentryHook) Levels() []logrus.Level {
	return h.levels
}

func (h *SentryHook) Fire(entry *logrus.Entry) error {
	// TODO: 集成 Sentry SDK
	// sentry.CaptureException(entry)
	return nil
}

// ElasticsearchHook Elasticsearch 日志存储 Hook
type ElasticsearchHook struct {
	client interface{} // Elasticsearch client
	index  string
	levels []logrus.Level
}

func NewElasticsearchHook(client interface{}, index string) *ElasticsearchHook {
	return &ElasticsearchHook{
		client: client,
		index:  index,
		levels: logrus.AllLevels,
	}
}

func (h *ElasticsearchHook) Levels() []logrus.Level {
	return h.levels
}

func (h *ElasticsearchHook) Fire(entry *logrus.Entry) error {
	// TODO: 将日志发送到 Elasticsearch
	// h.client.Index(h.index, entry)
	return nil
}

// CallerHook 修正调用者信息的 Hook（跳过 logger 包装层）
type CallerHook struct {
	skip int
}

func NewCallerHook(skip int) *CallerHook {
	return &CallerHook{skip: skip}
}

func (h *CallerHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *CallerHook) Fire(entry *logrus.Entry) error {
	// 获取真实的调用者信息
	// 从调用栈中查找第一个非 logger 包的调用者
	pcs := make([]uintptr, 30)
	depth := runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs[:depth])

	for {
		frame, more := frames.Next()
		// 跳过以下文件：
		// 1. logger 包内部文件（logrus.go, zap.go, default.go 等）
		// 2. logrus 库内部文件（entry.go, logger.go 等）
		// 3. runtime 包文件
		// 4. testing 包文件（仅在 Go test 运行时）
		if !strings.Contains(frame.File, "/logger/") &&
			!strings.Contains(frame.File, "/logrus@") &&
			!strings.Contains(frame.File, "/runtime/") &&
			!strings.Contains(frame.File, "/testing/") &&
			!strings.Contains(frame.Function, "github.com/zentic-org/go-admin-core/logger") &&
			!strings.Contains(frame.Function, "github.com/sirupsen/logrus") &&
			!strings.Contains(frame.Function, "runtime.") &&
			!strings.Contains(frame.Function, "testing.") {
			// 找到第一个业务代码调用位置
			entry.Caller = &runtime.Frame{
				PC:       frame.PC,
				File:     frame.File,
				Line:     frame.Line,
				Function: frame.Function,
			}
			break
		}
		if !more {
			break
		}
	}
	return nil
}

// MetricsHook Prometheus 监控 Hook
type MetricsHook struct {
	// TODO: Prometheus metrics
	levels []logrus.Level
}

func NewMetricsHook() *MetricsHook {
	return &MetricsHook{
		levels: logrus.AllLevels,
	}
}

func (h *MetricsHook) Levels() []logrus.Level {
	return h.levels
}

func (h *MetricsHook) Fire(entry *logrus.Entry) error {
	// TODO: 记录 Prometheus 指标
	// logCounter.WithLabelValues(entry.Level.String()).Inc()
	return nil
}
