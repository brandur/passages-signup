package main

import (
	"time"

	"gopkg.in/mailgun/mailgun-go.v1"
)

type MailAPI interface {
	AddMember(list, email string) error
	SendMessage(email, contents string) error
}

type FakeMailAPI struct {
}

func (a *FakeMailAPI) AddMember(list, email string) error {
	return nil
}

func (a *FakeMailAPI) SendMessage(email, contents string) error {
	return nil
}

type MailgunAPI struct {
	mg mailgun.Mailgun
}

func NewMailgunAPI(mailDomain, apiKey string) *MailgunAPI {
	return &MailgunAPI{
		mg: mailgun.NewMailgun(mailDomain, apiKey, ""),
	}
}

func (a *MailgunAPI) AddMember(list, email string) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05-0700")
	return a.mg.CreateMember(true, list, mailgun.Member{
		Address: email,
		Vars: map[string]interface{}{
			"passages-signup":           true,
			"passages-signup-timestamp": timestamp,
		},
	})
}

func (a *MailgunAPI) SendMessage(email, contents string) error {
	return nil
}
