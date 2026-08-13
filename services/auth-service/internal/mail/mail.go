package mail

import (
	"fmt"
	"net/smtp"
)

type Sender struct {
	addr string
	from string
}

func NewSender(addr, from string) *Sender {
	return &Sender{addr: addr, from: from}
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
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", s.from, to, subject, body)

	return smtp.SendMail(s.addr, nil, s.from, []string{to}, []byte(msg))
}
