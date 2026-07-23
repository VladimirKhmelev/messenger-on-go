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

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", s.from, to, subject, body)

	return smtp.SendMail(s.addr, nil, s.from, []string{to}, []byte(msg))
}
