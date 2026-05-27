package config

import (
	"os"
	"github.com/zentic-org/go-admin-core/logger"
	"github.com/zentic-org/go-admin-core/sdk/pkg"
	log "github.com/zentic-org/go-admin-core/logger"
)

type Logger struct {
	Type      string
	Path      string
	Level     string
	Stdout    string
	EnabledDB bool
	Cap       uint
}

// Setup 设置logger（使用新的 logger 架构）
func (e Logger) Setup() {
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
		logger.WithName("go-admin"),
		logger.WithCallerSkipCount(2),
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

	// 选择适配器
	switch e.Type {
	case "zap":
		log.DefaultLogger = logger.NewZapLogger(opts...)
	default:
		log.DefaultLogger = logger.NewLogrusLogger(opts...)
	}
}

var LoggerConfig = new(Logger)
