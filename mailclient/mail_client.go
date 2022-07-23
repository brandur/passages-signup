package mailclient

import (
	"context"
	"errors"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/mailgun/mailgun-go/v3"
	"github.com/sirupsen/logrus"
	"golang.org/x/xerrors"
)

var validate = validator.New()

//
// API
//

// API provides an abstract interface for a mailing service. It's useful
// for selecting between a real mailing service and fake one that's useful for
// development and testing.
type API interface {
	// AddMember adds a new member to a mailing list.
	AddMember(ctx context.Context, list, email string) error

	// SendMessage sends a message an email address.
	SendMessage(ctx context.Context, params *SendMessageParams) error
}

type SendMessageParams struct {
	ContentsHTML   string `validate:"required"`
	ContentsPlain  string `validate:"required"`
	ListAddress    string `validate:"required"`
	NewsletterName string `validate:"required"`
	Recipient      string `validate:"required"`
	ReplyTo        string `validate:"required"`
	Subject        string `validate:"required"`
}

//
// FakeClient
//

// FakeClient is a really primitive mock that we can use to verify that
// certain mail-related calls were made without reaching out to Mailgun.
type FakeClient struct {
	MembersAdded []*FakeClientAPIMemberAdded
	MessagesSent []*FakeClientAPIMessageSent
}

// FakeClientAPIMemberAdded records a mailing list member being added to a
// FakeClient.
type FakeClientAPIMemberAdded struct {
	List, Email string
}

// FakeClientAPIMessageSent records a message being sent from a FakeClient.
type FakeClientAPIMessageSent struct {
	ContentsHTML  string
	ContentsPlain string
	Recipient     string
	Subject       string
}

// NewFakeClient initializes a new FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{}
}

// AddMember adds a new member to a mailing list.
func (a *FakeClient) AddMember(ctx context.Context, list, email string) error {
	a.MembersAdded = append(a.MembersAdded,
		&FakeClientAPIMemberAdded{list, email})
	return nil
}

// SendMessage sends a message an email address.
func (a *FakeClient) SendMessage(ctx context.Context, params *SendMessageParams) error {
	if err := validate.Struct(params); err != nil {
		return xerrors.Errorf("error validating params: %w", err)
	}

	a.MessagesSent = append(a.MessagesSent,
		&FakeClientAPIMessageSent{
			ContentsHTML:  params.ContentsHTML,
			ContentsPlain: params.ContentsPlain,
			Recipient:     params.Recipient,
			Subject:       params.Subject,
		})

	return nil
}

//
// MailgunClient
//

// MailgunClient is an implementation of API that uses Mailgun (a third party
// mailing service).
type MailgunClient struct {
	mg mailgun.Mailgun
}

// NewMailgunClient initializes a new MailgunAPI with the given mailing domain and
// API key.
func NewMailgunClient(mailDomain, apiKey string) *MailgunClient {
	return &MailgunClient{
		mg: mailgun.NewMailgun(mailDomain, apiKey),
	}
}

// AddMember adds a new member to a mailing list.
func (a *MailgunClient) AddMember(ctx context.Context, list, email string) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05-0700")
	err := a.mg.CreateMember(ctx, true, list, mailgun.Member{
		Address: email,
		Vars: map[string]interface{}{
			"passages-signup":           true,
			"passages-signup-timestamp": timestamp,
		},
	})
	return interpretMailgunError(err)
}

// SendMessage sends a message an email address.
func (a *MailgunClient) SendMessage(ctx context.Context, params *SendMessageParams) error {
	if err := validate.Struct(params); err != nil {
		return xerrors.Errorf("error validating params: %w", err)
	}

	message := a.mg.NewMessage(
		params.NewsletterName+" <"+params.ListAddress+">",
		params.Subject,
		params.ContentsPlain)

	if err := message.AddRecipient(params.Recipient); err != nil {
		return xerrors.Errorf("error adding recipient: %w", err)
	}

	message.SetHtml(params.ContentsHTML)
	message.SetReplyTo(params.ReplyTo)

	resp, _, err := a.mg.Send(ctx, message)
	wrappedErr := xerrors.Errorf("error sending message: %w", err)
	logrus.Infof(`Sent to: %s (response: "%s") (error: "%s")`,
		params.Recipient, resp, wrappedErr)

	return wrappedErr
}

//
// Private functions
//

func interpretMailgunError(err error) error {
	var unexpectedErr *mailgun.UnexpectedResponseError
	if errors.As(err, &unexpectedErr) {
		message := string(unexpectedErr.Data)
		if message == "" {
			message = "(empty)"
		}

		return xerrors.Errorf("Got unexpected status code %v from Mailgun. Message: %v",
			unexpectedErr.Actual, message)
	}

	return err
}
