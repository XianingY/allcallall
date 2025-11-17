package mail

import (
	"crypto/tls"
	"fmt"

	"github.com/rs/zerolog"
	"gopkg.in/mail.v2"
)

// Config 邮件服务配置
// Config holds SMTP server settings
type Config struct {
	Host             string
	Port             int
	Username         string
	Password         string
	From             string
	FromName         string
	MaxRetries       int
	RetryDelaySecond int
}

// Service 邮件发送服务
// Service handles email operations via SMTP
type Service struct {
	config Config
	logger zerolog.Logger
}

// NewService 创建邮件服务
// NewService creates a new mail service instance
func NewService(cfg Config, logger zerolog.Logger) *Service {
	return &Service{
		config: cfg,
		logger: logger.With().Str("component", "mail_service").Logger(),
	}
}

// SendVerificationCode 发送验证码邮件
// SendVerificationCode sends a verification code email
func (s *Service) SendVerificationCode(email, code string) error {
	subject := "AllCallAll 邮箱验证码 / Email Verification Code"
	body := fmt.Sprintf(`
		<html>
			<body style="font-family: Arial, sans-serif; color: #333; background-color: #f5f5f5;">
				<div style="max-width: 600px; margin: 0 auto; padding: 20px; background-color: white; border-radius: 8px;">
					<h2 style="color: #1f2937; text-align: center;">邮箱验证 / Email Verification</h2>
					<p style="color: #6b7280; font-size: 14px;">您好，</p>
					<p style="color: #6b7280;">感谢您注册 AllCallAll。请使用以下验证码完成邮箱验证：</p>
					
					<div style="background-color: #f0f4f8; padding: 30px; text-align: center; margin: 20px 0; border-radius: 8px;">
						<h1 style="color: #2563eb; letter-spacing: 10px; margin: 0; font-size: 48px;">%s</h1>
						<p style="color: #6b7280; margin-top: 10px; font-size: 12px;">验证码有效期：10 分钟</p>
					</div>
					
					<p style="color: #6b7280; font-size: 14px;">请注意：</p>
					<ul style="color: #6b7280; font-size: 14px;">
						<li>不要与他人分享此验证码</li>
						<li>AllCallAll 工作人员不会要求您提供验证码</li>
						<li>如果这不是您的请求，请忽略此邮件</li>
					</ul>
					
					<hr style="border: none; border-top: 1px solid #e5e7eb; margin: 20px 0;">
					<p style="color: #9ca3af; font-size: 12px; text-align: center;">
						© 2024 AllCallAll. 保留所有权利。<br>
						实时音视频通信平台
					</p>
				</div>
			</body>
		</html>
	`, code)

	return s.send(email, subject, body)
}

// send 发送邮件（内部方法）
// send is an internal method to send emails via SMTP
func (s *Service) send(to, subject, body string) error {
	m := mail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s <%s>", s.config.FromName, s.config.From))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	// 创建 SMTP 拨号器
	d := mail.NewDialer(s.config.Host, s.config.Port, s.config.Username, s.config.Password)

	// 配置 TLS（Gmail 需要 TLS）
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         s.config.Host,
	}

	// 发送邮件
	if err := d.DialAndSend(m); err != nil {
		s.logger.Error().
			Err(err).
			Str("to", to).
			Str("subject", subject).
			Msg("failed to send email")
		return fmt.Errorf("send email: %w", err)
	}

	s.logger.Info().
		Str("to", to).
		Str("subject", subject).
		Msg("email sent successfully")

	return nil
}

// HealthCheck 检查 SMTP 连接
// HealthCheck verifies SMTP server connectivity
func (s *Service) HealthCheck() error {
	d := mail.NewDialer(s.config.Host, s.config.Port, s.config.Username, s.config.Password)
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         s.config.Host,
	}

	conn, err := d.Dial()
	if err != nil {
		s.logger.Error().Err(err).Msg("SMTP health check failed")
		return fmt.Errorf("smtp health check: %w", err)
	}
	defer conn.Close()

	s.logger.Info().Msg("SMTP health check passed")
	return nil
}
