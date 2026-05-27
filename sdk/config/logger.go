package config

import (
	"os"
	"github.com/zentic-org/go-admin-core/logger"
	"github.com/zentic-org/go-admin-core/sdk/pkg"
	log "github.com/zentic-org/go-admin-core/logger"
)

type Logger struct {
	Type      string
	Name      string // Logger 名称
	Path      string
	Level     string
	Stdout    string
	EnabledDB bool
	Cap       uint
	// 日志轮转配置
	MaxSize    int  // 单文件最大大小（MB）
	MaxAge     int  // 最大保留天数
	MaxBackups int  // 最大备份数量
	Compress   bool // 是否压缩
	LocalTime  bool // 是否使用本地时间
}

// Setup 设置logger（使用新的 logger 架构）
func (e Logger) Setup() {
	// 设置默认值
	if e.MaxSize == 0 {
		e.MaxSize = 100 // 默认 100MB
	}
	if e.MaxAge == 0 {
		e.MaxAge = 30 // 默认 30天
	}
	if e.MaxBackups == 0 {
		e.MaxBackups = 7 // 默认 7个备份
	}
	// 默认启用压缩和本地时间
	if !e.Compress {
		e.Compress = true
	}
	if !e.LocalTime {
		e.LocalTime = true
	}
	
	// 确保目录存在
	if e.Path != "" {
		// 检查路径是否存在且是文件，如果是文件则删除
		if info, err := os.Stat(e.Path); err == nil && !info.IsDir() {
			log.Warnf("log path %s is a file, removing it", e.Path)
			if err := os.Remove(e.Path); err != nil {
				log.Fatalf("failed to remove log file: %s", err.Error())
			}
		}
		
		if !pkg.PathExist(e.Path) {
			if err := pkg.PathCreate(e.Path); err != nil {
				log.Fatalf("create log dir error: %s", err.Error())
			}
		}
	}

	// 构建选项
	opts := []logger.Option{
		logger.WithCallerSkipCount(2),
	}
	
	// 设置 Logger 名称（允许自定义）
	if e.Name != "" {
		opts = append(opts, logger.WithName(e.Name))
	} else {
		opts = append(opts, logger.WithName("go-admin")) // 默认名称
	}

	// 设置日志级别
	if level, err := logger.GetLevel(e.Level); err == nil {
		opts = append(opts, logger.WithLevel(level))
	}

	// 设置输出
	if e.Stdout == "file" || e.Stdout == "" {
		opts = append(opts, logger.WithStdout(false))
	} else {
		opts = append(opts, logger.WithStdout(true))
	}

	// 设置文件路径
	if e.Path != "" {
		opts = append(opts, logger.WithPath(e.Path))
	}

	// 设置日志轮转配置
	if e.MaxSize > 0 {
		opts = append(opts, logger.WithMaxSize(e.MaxSize))
	}
	if e.MaxAge > 0 {
		opts = append(opts, logger.WithMaxAge(e.MaxAge))
	}
	if e.MaxBackups > 0 {
		opts = append(opts, logger.WithMaxBackups(e.MaxBackups))
	}
	// 默认启用压缩
	opts = append(opts, logger.WithCompress(e.Compress))
	opts = append(opts, logger.WithLocalTime(e.LocalTime))

	// 选择适配器
	switch e.Type {
	case "zap":
		log.DefaultLogger = logger.NewZapLogger(opts...)
	default:
		log.DefaultLogger = logger.NewLogrusLogger(opts...)
	}
}

var LoggerConfig = new(Logger)
