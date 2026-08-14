package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type Sender interface {
	Enabled() bool
	Send(ctx context.Context, to, subject, body string) error
}

type SMTP struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

func FromEnv() *SMTP {
	host := strings.TrimSpace(os.Getenv("LEDGER_SMTP_HOST"))
	user := strings.TrimSpace(os.Getenv("LEDGER_SMTP_USER"))
	pass := os.Getenv("LEDGER_SMTP_PASS")
	from := strings.TrimSpace(os.Getenv("LEDGER_SMTP_FROM"))
	if from == "" {
		from = user
	}
	port := 465
	if p := strings.TrimSpace(os.Getenv("LEDGER_SMTP_PORT")); p != "" {
		n, err := strconv.Atoi(p)
		if err == nil && n > 0 {
			port = n
		}
	}
	if host == "" || user == "" || pass == "" || from == "" {
		return nil
	}
	return &SMTP{Host: host, Port: port, User: user, Pass: pass, From: from}
}

func (s *SMTP) Enabled() bool { return s != nil && s.Host != "" && s.User != "" && s.Pass != "" }

func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("smtp not configured")
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	msg := strings.Join([]string{
		"From: " + s.From,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	var (
		conn net.Conn
		err  error
	)
	if s.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if s.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		}
	}
	if err := c.Auth(smtp.PlainAuth("", s.User, s.Pass, s.Host)); err != nil {
		return err
	}
	if err := c.Mail(s.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
