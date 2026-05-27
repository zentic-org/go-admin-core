package config

type Application struct {
	ReadTimeout   int
	WriterTimeout int
	Host          string
	Port          int64
	Name          string
	Mode          string
	DemoMsg       string
	EnableDP      bool
	// 租户模式：1：单租户 2：多租户
	TenantMode int `yaml:"tenantMode"`
}

var ApplicationConfig = new(Application)
