package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mailgun/mailgun-go/v3"
)

//
// MailAPI
//

// MailAPI provides an abstract interface for a mailing service. It's useful
// for selecting between a real mailing service and fake one that's useful for
// development and testing.
type MailAPI interface {
	// AddMember adds a new member to a mailing list.
	AddMember(list, email string) error

	// SendMessage sends a message an email address.
	SendMessage(email, subject, contentsPlain, contentsHTML string) error
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

// FakeMailAPIMemberAdded records a mailing list member being added to a
// FakeMailAPI.
type FakeMailAPIMemberAdded struct {
	List, Email string
}

// FakeMailAPIMessageSent records a message being sent from a FakeMailAPI.
type FakeMailAPIMessageSent struct {
	Email, Subject, ContentsPlain, ContentsHTML string
}

// NewFakeMailAPI initializes a new FakeMailAPI.
func NewFakeMailAPI() *FakeMailAPI {
	return &FakeMailAPI{}
}

// AddMember adds a new member to a mailing list.
func (a *FakeMailAPI) AddMember(list, email string) error {
	a.MembersAdded = append(a.MembersAdded,
		&FakeMailAPIMemberAdded{list, email})
	return nil
}

// SendMessage sends a message an email address.
func (a *FakeMailAPI) SendMessage(email, subject, contentsPlain, contentsHTML string) error {
	a.MessagesSent = append(a.MessagesSent,
		&FakeMailAPIMessageSent{email, subject, contentsPlain, contentsHTML})
	return nil
}

//
// MailgunAPI
//

// MailgunAPI is an implementation of MailAPI that uses Mailgun (a third party
// mailing service).
type MailgunAPI struct {
	mg mailgun.Mailgun
}

// NewMailgunAPI initializes a new MailgunAPI with the given mailing domain and
// API key.
func NewMailgunAPI(mailDomain, apiKey string) *MailgunAPI {
	return &MailgunAPI{
		mg: mailgun.NewMailgun(mailDomain, apiKey),
	}
}

// AddMember adds a new member to a mailing list.
func (a *MailgunAPI) AddMember(list, email string) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05-0700")
	err := a.mg.CreateMember(context.Background(), true, list, mailgun.Member{
		Address: email,
		Vars: map[string]interface{}{
			"passages-signup":           true,
			"passages-signup-timestamp": timestamp,
		},
	})
	return interpretMailgunError(err)
}

// SendMessage sends a message an email address.
func (a *MailgunAPI) SendMessage(email, subject, contentsPlain, contentsHTML string) error {
	message := a.mg.NewMessage(
		string(conf.newsletterName)+" <"+conf.listAddress+">",
		subject,
		contentsPlain)
	message.AddRecipient(email)
	message.SetHtml(contentsHTML)
	message.SetReplyTo(replyToAddress)

	resp, _, err := a.mg.Send(context.Background(), message)
	log.Printf(`Sent to: %s (response: "%s") (error: "%s")`,
		email, resp, err)

	return err
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
