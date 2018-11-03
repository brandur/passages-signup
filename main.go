package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/brandur/csrf"
	"github.com/gorilla/mux"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

const (
	envProduction = "production"
	envTesting    = "testing"

	fromAddress    = "Passages & Glass Bot <" + listAddress + ">"
	listAddress    = "passages@" + mailDomain
	mailDomain     = "list.brandur.org"
	mailList       = "passages@" + mailDomain
	replyToAddress = "brandur@brandur.org"
)

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// DatabaseURL is the URL to the Postgres database used to store program
	// state.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// MailgunAPIKey is a key for Mailgun used to send email.
	MailgunAPIKey string `env:"MAILGUN_API_KEY,required"`

	// PassagesEnv determines the running environment of the app. Set to
	// development to disable template caching and CSRF protection.
	PassagesEnv string `env:"PASSAGES_ENV,default=production"`

	// Port is the port over which to serve HTTP.
	Port string `env:"PORT,default=5001"`

	// PublicDir is the local directory out of which static content (images,
	// stylesheets, etc.) will be served.
	PublicDir string `env:"PUBLIC_DIR,default=./public"`

	// PublicURL is the public location from which the site is being served.
	// This is needed in some places to generate absolute URLs.
	PublicURL string `env:"PUBLIC_URL,default=https://passages-signup.herokuapp.com"`
}

var conf Conf

func main() {
	err := envdecode.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/", handleShow)
	r.HandleFunc("/confirm/{token}", handleConfirm)
	r.HandleFunc("/submit", handleSubmit)

	if conf.PassagesEnv != envProduction {
		r.HandleFunc("/messages/confirm", handleShowConfirmMessagePreview)
		r.HandleFunc("/messages/confirm_plain", handleShowConfirmMessagePlainPreview)
	}

	// Serves up static files found in public/
	r.PathPrefix("/public/").Handler(
		http.StripPrefix("/public/", http.FileServer(http.Dir(conf.PublicDir))),
	)

	var handler http.Handler = r

	options := []csrf.Option{
		csrf.AllowedOrigin("https://brandur.org"),
		csrf.AllowedOrigin("https://passages-signup.herokuapp.com"),
	}

	if conf.PassagesEnv != envProduction {
		log.Printf("Allowing localhost origin for non-production environment")
		options = append(options,
			csrf.AllowedOrigin("http://localhost:"+conf.Port))

	}
	handler = csrf.Protect(options...)(handler)

	if conf.PassagesEnv == envProduction {
		handler = redirectToHTTPS(handler)
	}

	log.Printf("Listening on port %v", conf.Port)
	log.Fatal(http.ListenAndServe(":"+conf.Port, handler))
}

//
// Handlers ---
//

func handleConfirm(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		vars := mux.Vars(r)
		token := vars["token"]

		db, err := sql.Open("postgres", conf.DatabaseURL)
		if err != nil {
			return err
		}

		var res *SignupFinisherResult
		err = WithTransaction(db, func(tx *sql.Tx) error {
			mediator := &SignupFinisher{
				MailAPI: getMailAPI(),
				Token:   token,
			}

			var err error
			res, err = mediator.Run(tx)
			return err
		})

		var message string
		if err != nil {
			return errors.Wrap(err, "Encountered a problem finishing signup")
		}

		if res.TokenNotFound {
			w.WriteHeader(http.StatusNotFound)
			message = "We couldn't find that confirmation token."
		} else {
			message = fmt.Sprintf("Thank you for signing up. You'll receive your first newsletter at <strong>%s</strong> the next time an edition of <em>Passages & Glass</em> is published.", res.Email)
		}

		return renderTemplate(w, "views/message", getLocals(map[string]interface{}{
			"message": message,
		}))
	})
}

func handleShow(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		return renderTemplate(w, "views/show", getLocals(map[string]interface{}{}))
	})
}

func handleShowConfirmMessagePreview(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		return renderTemplate(w, "views/messages/confirm", getLocals(map[string]interface{}{
			"token": "bc492bd9-2aea-458a-aea1-cd7861c334d1",
		}))
	})
}

func handleShowConfirmMessagePlainPreview(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		return renderTemplate(w, "views/messages/confirm_plain", getLocals(map[string]interface{}{
			"token": "bc492bd9-2aea-458a-aea1-cd7861c334d1",
		}))
	})
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		// Only accept form POSTs.
		if r.Method != "POST" {
			http.NotFound(w, r)
			return nil
		}

		err := r.ParseForm()
		if err != nil {
			renderError(w, http.StatusBadRequest,
				errors.Wrap(err, "Unable to parse input form"))
			return nil
		}

		email := r.Form.Get("email")
		if email == "" {
			renderError(w, http.StatusUnprocessableEntity,
				fmt.Errorf("Expected input parameter email"))
			return nil
		}

		email = strings.TrimSpace(email)

		db, err := sql.Open("postgres", conf.DatabaseURL)
		if err != nil {
			return err
		}

		var res *SignupStarterResult
		err = WithTransaction(db, func(tx *sql.Tx) error {
			mediator := &SignupStarter{
				Email:   email,
				MailAPI: getMailAPI(),
			}

			var err error
			res, err = mediator.Run(tx)
			return err
		})

		var message string
		if err != nil {
			return errors.Wrap(err, "Encountered a problem sending a confirmation email")
		}

		if res.AlreadySubscribed {
			message = fmt.Sprintf("<strong>%s</strong> is already subscribed to <em>Passages & Glass</em>. Thank you for signing up!", email)
		} else if res.ConfirmationRateLimited {
			message = fmt.Sprintf("Thank you for signing up. We recently sent a confirmation email to <strong>%s</strong> and don't want to send another one so soon. Please try to find the message and click the enclosed link to finish signing up for <em>Passages & Glass</em> (check your spam folder if you can't find it).", email)
		} else {
			message = fmt.Sprintf("Thank you for signing up. We've sent a confirmation email to <strong>%s</strong>. Please click the enclosed link to finish signing up for <em>Passages & Glass</em>.", email)
		}

		return renderTemplate(w, "views/message", getLocals(map[string]interface{}{
			"message": message,
		}))
	})
}

//
// Private functions
//

// getMailAPI gets a mailing API appropriate for the current environment. If
// we're in testing, we create a fake API so that we never make any real calls
// out to Mailgun.
func getMailAPI() MailAPI {
	if conf.PassagesEnv == envTesting {
		return NewFakeMailAPI()
	}

	return NewMailgunAPI(mailDomain, conf.MailgunAPIKey)
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

func renderError(w http.ResponseWriter, status int, renderErr error) {
	w.WriteHeader(status)

	err := renderTemplate(w, "views/error", getLocals(map[string]interface{}{
		"error": renderErr.Error(),
	}))
	if err != nil {
		// Hopefully it never comes to this
		log.Printf("Error during error handling: %v", err)
	}
}

func withErrorHandling(w http.ResponseWriter, fn func() error) {
	err := fn()
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
}
