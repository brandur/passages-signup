package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/joeshaw/envdecode"
	"github.com/yosssi/ace"
	"gopkg.in/mailgun/mailgun-go.v1"
)

const (
	envProduction = "production"
	envTesting    = "testing"
	mailDomain    = "list.brandur.org"
	mailList      = "passages@" + mailDomain
)

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// CSRF is a 32-byte secret used for CSRF protection.
	CSRFSecret string `env:"CSRF_SECRET,required"`

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

	r := mux.NewRouter()
	r.HandleFunc("/", handleShow)
	r.HandleFunc("/submit", handleSubmit)
	var handler http.Handler = r

	if conf.PassagesEnv != envProduction {
		log.Printf("Setting CSRF secure to false for non-production environment")
		// When developing locally we're generally using HTTP and Gorilla will
		// not set its cookie on this insecure channel. We override this here
		// to allow CSRF protection to work.
		csrf.Secure(false)
	}
	csrfMiddleware := csrf.Protect([]byte(conf.CSRFSecret))
	handler = csrfMiddleware(handler)

	if conf.PassagesEnv == envProduction {
		handler = redirectToHTTPS(handler)
	}

	log.Printf("Listening on port %v", conf.Port)
	log.Fatal(http.ListenAndServe(":"+conf.Port, handler))
}

//
// Handlers ---
//

func handleShow(w http.ResponseWriter, r *http.Request) {
	template, err := getTemplate("views/show")
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}

	locals := map[string]interface{}{
		// Value to render as the form's CSRF token
		csrf.TemplateTag: csrf.TemplateField(r),
	}
	err = template.Execute(w, locals)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
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

	var message string
	if conf.PassagesEnv != envTesting {
		mg := mailgun.NewMailgun(mailDomain, conf.MailgunAPIKey, "")
		timestamp := time.Now().UTC().Format("2006-01-02T15:04:05-0700")
		err = mg.CreateMember(true, mailList, mailgun.Member{
			Address: email,
			Vars: map[string]interface{}{
				"passages-signup":           true,
				"passages-signup-timestamp": timestamp,
			},
		})

		if err != nil {
			errStr := interpretMailgunError(err)
			log.Printf(errStr)
			message = fmt.Sprintf("We ran into a problem adding you to the list: %v",
				errStr)
		}
	} else {
		message = "Skipped Mailgun access for testing"
		log.Printf(message)
	}

	locals := map[string]interface{}{
		"email":   email,
		"message": message,
	}
	err = template.Execute(w, locals)
	if err != nil {
		// Body may have already been sent, so just respond normally.
		log.Printf("Error rendering template: %v", err)
		return
	}
}

//
// Helpers ---
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

func redirectToHTTPS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		proto := req.Header.Get("X-Forwarded-Proto")
		if proto == "http" || proto == "HTTP" {
			http.Redirect(res, req,
				fmt.Sprintf("https://%s%s", req.Host, req.URL),
				http.StatusPermanentRedirect)
			return
		}

		next.ServeHTTP(res, req)

	})
}

func renderError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	message := fmt.Sprintf("Error %v", status)
	if err != nil {
		message = fmt.Sprintf("%v: %v", message, err.Error())
	}
	w.Write([]byte(message))
}
