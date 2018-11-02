package main

import (
	"fmt"
	"time"

	"gopkg.in/mailgun/mailgun-go.v1"
)

//
// MailAPI
//

type MailAPI interface {
	AddMember(list, email string) error
	SendMessage(email, contents string) error
}

//
// FakeMailAPI
//

// FakeMailAPI is a really primitive mock that we can use to verify that
// certain mail-related calls were made without reaching out to Mailgun.
type FakeMailAPI struct {
	MembersAdded []*FakeMailAPIMemberAdded
	MessagesSent []*FakeMailAPIMessageSent
}

type FakeMailAPIMemberAdded struct {
	List, Email string
}

type FakeMailAPIMessageSent struct {
	Email, Contents string
}

func NewFakeMailAPI() *FakeMailAPI {
	return &FakeMailAPI{}
}

func (a *FakeMailAPI) AddMember(list, email string) error {
	a.MembersAdded = append(a.MembersAdded,
		&FakeMailAPIMemberAdded{list, email})
	return nil
}

func (a *FakeMailAPI) SendMessage(email, contents string) error {
	a.MessagesSent = append(a.MessagesSent,
		&FakeMailAPIMessageSent{email, contents})
	return nil
}

//
// MailgunAPI
//

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
	err := a.mg.CreateMember(true, list, mailgun.Member{
		Address: email,
		Vars: map[string]interface{}{
			"passages-signup":           true,
			"passages-signup-timestamp": timestamp,
		},
	})
	return interpretMailgunError(err)
}

func (a *MailgunAPI) SendMessage(email, contents string) error {
	return nil
}

//
// Private functions
//

func interpretMailgunError(err error) error {
	unexpectedErr, ok := err.(*mailgun.UnexpectedResponseError)
	if ok {
		message := string(unexpectedErr.Data)
		if message == "" {
			message = "(empty)"
		}

		return fmt.Errorf("Got unexpected status code %v from Mailgun. Message: %v",
			unexpectedErr.Actual, message)
	}

	return err
}
