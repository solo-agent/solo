package authmail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/solo-ai/solo/pkg/config"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
)

type Sender struct {
	cfg *config.Config
}

func NewSender(cfg *config.Config) *Sender {
	return &Sender{cfg: cfg}
}

func (s *Sender) SendCode(ctx context.Context, recipient, code, purpose string) error {
	if s.cfg.AuthMailTransport == "tencent_ses" {
		return s.sendTencentSES(ctx, recipient, code, purpose)
	}
	if s.cfg.SMTPHost == "" {
		slog.Info("development auth email", "email", recipient, "purpose", purpose, "code", code)
		return nil
	}

	to, err := mail.ParseAddress(recipient)
	if err != nil || !strings.EqualFold(to.Address, recipient) {
		return fmt.Errorf("invalid recipient")
	}
	from, err := mail.ParseAddress(s.cfg.SMTPFrom)
	if err != nil {
		return fmt.Errorf("invalid SMTP_FROM: %w", err)
	}

	subject, intro := codeMessage(purpose)
	body := fmt.Sprintf("<p>%s</p><p style=\"font-size:28px;font-weight:700;letter-spacing:6px\">%s</p><p>This code expires in 10 minutes. If you did not request it, you can ignore this email.</p>", html.EscapeString(intro), html.EscapeString(code))

	addr := net.JoinHostPort(s.cfg.SMTPHost, s.cfg.SMTPPort)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.cfg.SMTPTLS == "implicit" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connect SMTP: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	client, err := smtp.NewClient(conn, s.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("start SMTP: %w", err)
	}
	defer client.Close()

	if s.cfg.SMTPTLS != "implicit" {
		ok, _ := client.Extension("STARTTLS")
		if !ok && s.cfg.SMTPHost != "localhost" && s.cfg.SMTPHost != "127.0.0.1" {
			return fmt.Errorf("SMTP server does not offer STARTTLS")
		}
		if ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("start SMTP TLS: %w", err)
			}
		}
	}
	if s.cfg.SMTPUsername != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)); err != nil {
			return fmt.Errorf("authenticate SMTP: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("SMTP sender: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("SMTP recipient: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP data: %w", err)
	}
	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s", from.String(), to.String(), subject, body)
	if _, err := w.Write(message.Bytes()); err != nil {
		_ = w.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	return client.Quit()
}

func (s *Sender) sendTencentSES(ctx context.Context, recipient, code, purpose string) error {
	request, err := newTencentSESRequest(s.cfg, recipient, code, purpose)
	if err != nil {
		return err
	}
	client, err := ses.NewClient(
		common.NewCredential(s.cfg.TencentCloudSecretID, s.cfg.TencentCloudSecretKey),
		s.cfg.TencentSESRegion,
		profile.NewClientProfile(),
	)
	if err != nil {
		return fmt.Errorf("initialize Tencent SES: %w", err)
	}
	if _, err := client.SendEmailWithContext(ctx, request); err != nil {
		return fmt.Errorf("send Tencent SES email: %w", err)
	}
	return nil
}

func newTencentSESRequest(cfg *config.Config, recipient, code, purpose string) (*ses.SendEmailRequest, error) {
	to, err := mail.ParseAddress(recipient)
	if err != nil || !strings.EqualFold(to.Address, recipient) {
		return nil, fmt.Errorf("invalid recipient")
	}
	subject, intro := codeMessage(purpose)
	templateData, err := json.Marshal(map[string]string{"code": code, "intro": intro})
	if err != nil {
		return nil, fmt.Errorf("encode Tencent SES template data: %w", err)
	}
	request := ses.NewSendEmailRequest()
	request.FromEmailAddress = common.StringPtr(cfg.TencentSESFrom)
	request.Subject = common.StringPtr(subject)
	request.Destination = common.StringPtrs([]string{to.Address})
	request.Template = &ses.Template{
		TemplateID:   common.Uint64Ptr(uint64(cfg.TencentSESTemplateID)),
		TemplateData: common.StringPtr(string(templateData)),
	}
	request.TriggerType = common.Uint64Ptr(1)
	return request, nil
}

func codeMessage(purpose string) (subject, intro string) {
	if purpose == "password_reset" {
		return "Reset your Solo password", "Enter this code to reset your Solo password."
	}
	return "Verify your Solo email", "Enter this code to finish creating your Solo account."
}
