package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

type Sender struct {
	addr        string
	from        string
	displayName string
	auth        smtp.Auth
}

func NewSender(addr, from, displayName, username, password string) *Sender {
	var auth smtp.Auth
	if username != "" {
		host := addr
		if i := strings.LastIndex(addr, ":"); i != -1 {
			host = addr[:i]
		}
		auth = smtp.PlainAuth("", username, password, host)
	}
	return &Sender{addr: addr, from: from, displayName: displayName, auth: auth}
}

func (s *Sender) SendVerificationCode(to, code string) error {
	subject := "Your verification code"
	body := fmt.Sprintf("Your verification code is: %s\r\nIt expires in 10 minutes.", code)

	return s.send(to, subject, body)
}

func (s *Sender) SendPasswordResetToken(to, token string) error {
	subject := "Password reset"
	body := fmt.Sprintf("Your password reset token is: %s\r\nIt expires in 30 minutes.\r\nIf you did not request a password reset, ignore this email.", token)

	return s.send(to, subject, body)
}

func (s *Sender) SendPasswordChanged(to string) error {
	subject := "Your password was changed"
	body := "Your account password was just changed. If this wasn't you, reset your password immediately and contact support."

	return s.send(to, subject, body)
}

func (s *Sender) send(to, subject, body string) error {
	fromHeader := s.from
	if s.displayName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.displayName, s.from)
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", fromHeader, to, subject, body)

	return smtp.SendMail(s.addr, s.auth, s.from, []string{to}, []byte(msg))
}
