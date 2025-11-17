package config

// Mail 邮件配置
// Mail configuration for SMTP email service
type Mail struct {
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	Username         string `yaml:"username"`
	Password         string `yaml:"password"`
	From             string `yaml:"from"`
	FromName         string `yaml:"from_name"`
	MaxRetries       int    `yaml:"max_retries"`
	RetryDelaySecond int    `yaml:"retry_delay_seconds"`
}
