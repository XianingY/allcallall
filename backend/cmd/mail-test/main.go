package main

import (
"fmt"
"log"
"os"

"github.com/allcallall/backend/internal/config"
"github.com/allcallall/backend/internal/logger"
"github.com/allcallall/backend/internal/mail"
)

func main() {
	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	// 2. 初始化日志
	appLogger := logger.New(cfg.Logging.Level)

	// 3. 从环境变量读取邮箱密码
	mailPassword := os.Getenv("MAIL_PASSWORD")
	if mailPassword == "" {
		mailPassword = cfg.Mail.Password
	}

	if mailPassword == "" {
		log.Fatal("❌ MAIL_PASSWORD environment variable or config is not set")
	}

	appLogger.Info().Msg("=== AllCallAll Mail Test ===")
	appLogger.Info().
		Str("host", cfg.Mail.Host).
		Int("port", cfg.Mail.Port).
		Str("username", cfg.Mail.Username).
		Msg("SMTP Configuration")

	// 4. 创建邮件服务
	mailSvc := mail.NewService(mail.Config{
Host:             cfg.Mail.Host,
Port:             cfg.Mail.Port,
Username:         cfg.Mail.Username,
Password:         mailPassword,
From:             cfg.Mail.From,
FromName:         cfg.Mail.FromName,
MaxRetries:       cfg.Mail.MaxRetries,
RetryDelaySecond: cfg.Mail.RetryDelaySecond,
}, appLogger)

	appLogger.Info().Msg("✓ Mail service created successfully")

	// 5. 测试 SMTP 连接
	appLogger.Info().Msg("🔗 Testing SMTP connection...")
	if err := mailSvc.HealthCheck(); err != nil {
		appLogger.Error().Err(err).Msg("❌ SMTP health check failed")
		os.Exit(1)
	}
	appLogger.Info().Msg("✓ SMTP connection successful")

	// 6. 测试发送验证码邮件
	testEmail := os.Getenv("TEST_EMAIL")
	if testEmail == "" {
		testEmail = "allcallall.official@gmail.com"
	}

	testCode := "123456"

	appLogger.Info().
		Str("email", testEmail).
		Str("code", testCode).
		Msg("�� Sending verification code email...")

	if err := mailSvc.SendVerificationCode(testEmail, testCode); err != nil {
		appLogger.Error().Err(err).Msg("❌ Failed to send verification code")
		os.Exit(1)
	}

	appLogger.Info().
		Str("email", testEmail).
		Msg("✓ Verification code email sent successfully")

	// 7. 测试成功
	fmt.Println()
	fmt.Println("✅ All tests passed!")
	fmt.Println()
	fmt.Printf("📊 Test Summary:\n")
	fmt.Printf("   ✓ SMTP Configuration: %s:%d\n", cfg.Mail.Host, cfg.Mail.Port)
	fmt.Printf("   ✓ SMTP Connection: OK\n")
	fmt.Printf("   ✓ Email Sent: %s\n", testEmail)
	fmt.Printf("   ✓ Test Code: %s\n", testCode)
	fmt.Println()
	fmt.Println("📬 Check your inbox for the verification code email.")
}
