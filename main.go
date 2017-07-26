package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/joeshaw/envdecode"
	"github.com/yosssi/ace"
	"gopkg.in/mailgun/mailgun-go.v1"
)

const (
	envProduction = "production"
	mailDomain    = "list.brandur.org"
	mailList      = "passages@" + mailDomain
)

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// MailgunAPIKey is a key for Mailgun used to send email.
	MailgunAPIKey string `env:"MAILGUN_API_KEY,required"`

	// PassagesEnv determines the running environment of the app. Set to
	// development to disable template caching and CSRF protection.
	PassagesEnv string `env:"PASSAGES_ENV,default=production"`

	// Port is the port over which to serve HTTP.
	Port string `env:"PORT,default=5001"`
}

var conf Conf

func main() {
	err := envdecode.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		template, err := getTemplate("views/show")
		if err != nil {
			renderError(w, http.StatusInternalServerError, err)
			return
		}

		locals := map[string]interface{}{}
		err = template.Execute(w, locals)
		if err != nil {
			renderError(w, http.StatusInternalServerError, err)
			return
		}
	})
	http.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		// Only accept form POSTs.
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}

		err := r.ParseForm()
		if err != nil {
			renderError(w, http.StatusBadRequest, err)
		}

		email := r.Form.Get("email")
		if email == "" {
			renderError(w, http.StatusUnprocessableEntity,
				fmt.Errorf("Expected input parameter email"))
		}

		template, err := getTemplate("views/submit")
		if err != nil {
			renderError(w, http.StatusInternalServerError, err)
			return
		}

		mg := mailgun.NewMailgun(mailDomain, conf.MailgunAPIKey, "")
		err = mg.CreateMember(true, mailList, mailgun.Member{
			Address: email,
		})

		var message string
		if err != nil {
			errStr := interpretMailgunError(err)
			log.Printf(errStr)
			message = fmt.Sprintf("We ran into a problem adding you to the list: %v",
				errStr)
		}

		locals := map[string]interface{}{
			"Email":   email,
			"Message": message,
		}
		err = template.Execute(w, locals)
		if err != nil {
			// Body may have already been sent, so just respond normally.
			log.Printf("Error rendering template: %v", err)
			return
		}
	})
	log.Printf("Listening on port %v", conf.Port)
	log.Fatal(http.ListenAndServe(":"+conf.Port, nil))
}

//
// ---
//

func getTemplate(file string) (*template.Template, error) {
	if conf.PassagesEnv != envProduction {
		ace.FlushCache()
	}
	return ace.Load("layouts/main", file, nil)
}

func interpretMailgunError(err error) string {
	unexpectedErr, ok := err.(*mailgun.UnexpectedResponseError)
	if ok {
		message := string(unexpectedErr.Data)
		if message == "" {
			message = "(empty)"
		}

		return fmt.Sprintf("Got unexpected status code %v from Mailgun. Message: %v",
			unexpectedErr.Actual, message)
	}

	return err.Error()
}

func renderError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	message := fmt.Sprintf("Error %v", status)
	if err != nil {
		message = fmt.Sprintf("%v: %v", message, err.Error())
	}
	w.Write([]byte(message))
}
