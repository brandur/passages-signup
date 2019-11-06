package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/brandur/csrf"
	"github.com/gorilla/mux"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/memstore"
	"golang.org/x/crypto/acme/autocert"
)

const (
	envProduction = "production"
	envTesting    = "testing"

	mailDomain     = "list.brandur.org"
	replyToAddress = "brandur@brandur.org"
)

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// AssetsDir is the local directory out of which layouts, static content
	// (images, stylesheets, etc.), and views will be served.
	//
	// Must have a trailing slash.
	//
	// Defaults to the current directory.
	AssetsDir string `env:"ASSETS_DIR,default=./"`

	// DatabaseURL is the URL to the Postgres database used to store program
	// state.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// EnableLetsEncrypt causes the program to listen for HTTPS and
	// automatically provision a certificate through ACME/Let's Encrypt.
	//
	// If enabled, `PORT` is ignored and the program listens on 443.
	EnableLetsEncrypt bool `env:"ENABLE_LETS_ENCRYPT"`

	// MailgunAPIKey is a key for Mailgun used to send email.
	MailgunAPIKey string `env:"MAILGUN_API_KEY,required"`

	// Newsletter is the newsletter to send. Should be either `nanoglyph` or
	// `passages` and defaults to the latter. Along with one of the available
	// values it should also be the identifier of the list in Mailgun.
	NewsletterID NewsletterID `env:"NEWSLETTER_ID,default=passages"`

	// PassagesEnv determines the running environment of the app. Set to
	// development to disable template caching and CSRF protection.
	PassagesEnv string `env:"PASSAGES_ENV,default=production"`

	// Port is the port over which to serve HTTP.
	//
	// If `ENABLE_LETS_ENCRYPT` is enabled, this option is ignored.
	Port string `env:"PORT,default=5001"`

	// PublicURL is the public location from which the site is being served.
	// This is needed in some places to generate absolute URLs. Also used for
	// CSRF protection.
	PublicURL string `env:"PUBLIC_URL,default=https://passages-signup.herokuapp.com"`

	// Some newsletter-specific properties that are set based off the value of Newsletter.
	listAddress           string
	mailList              string
	newsletterName        NewsletterName
	newsletterDescription NewsletterDescription
}

// NewsletterID identifies a newsletter and its values are used as options for
// an incoming environmental variable.
type NewsletterID string

// NewsletterName represents the name of a newsletter.
type NewsletterName string

// NewsletterDescription represents the description of a newsletter.
type NewsletterDescription string

const (
	nanoglyphID          NewsletterID          = "nanoglyph"
	nanoglyphName        NewsletterName        = "Nanoglyph"
	nanoglyphDescription NewsletterDescription = NewsletterDescription(`<em>` + string(nanoglyphName) + `</em> is a TODO`)

	passagesID          NewsletterID          = "passages"
	passagesName        NewsletterName        = "Passages & Glass"
	passagesDescription NewsletterDescription = NewsletterDescription(`<em>` + string(passagesName) + `</em> is a personal newsletter about exploration, ideas, and software written by <a href="https://brandur.org">Brandur</a>. It's sent rarely – just a few times a year.`)
)

var conf Conf

func main() {
	err := envdecode.Decode(&conf)
	if err != nil {
		log.Fatal(err)
	}

	switch conf.NewsletterID {
	case nanoglyphID:
		conf.newsletterName = nanoglyphName
		conf.newsletterDescription = nanoglyphDescription

	case passagesID:
		conf.newsletterName = passagesName
		conf.newsletterDescription = passagesDescription

	default:
		log.Fatalf("Unknown newsletter configuration (`NEWSLETTER_ID`): %s (should be either %s or %s)",
			conf.NewsletterID, nanoglyphID, passagesID)
	}
	conf.listAddress = string(conf.NewsletterID) + "@" + mailDomain
	conf.mailList = string(conf.NewsletterID) + "@" + mailDomain

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
		http.StripPrefix("/public/", http.FileServer(http.Dir(conf.AssetsDir+"/public"))),
	)

	var handler http.Handler = r

	options := []csrf.Option{
		csrf.AllowedOrigin(conf.PublicURL),

		// And also allow the special origin from `brandur.org` which will
		// cross-post to this app.
		csrf.AllowedOrigin("https://brandur.org"),
	}

	if conf.PassagesEnv != envProduction {
		log.Printf("Allowing localhost origin for non-production environment")
		options = append(options,
			csrf.AllowedOrigin("http://localhost:"+conf.Port))

	}
	handler = csrf.Protect(options...)(handler)

	// Use a rate limiter to prevent enumeration of email addresses and so it's
	// harder to maliciously burn through my Mailgun API limit.
	rateLimiter, err := getRateLimiter()
	if err != nil {
		log.Fatal(err)
	}
	handler = rateLimiter.RateLimit(handler)

	if conf.PassagesEnv == envProduction {
		handler = redirectToHTTPS(handler)
	}

	if conf.EnableLetsEncrypt {
		go serveHTTPSRedirect()

		u, err := url.Parse(conf.PublicURL)
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("Listening on port 443 with automatic Let's Encrypt certificate")
		log.Fatal(http.Serve(autocert.NewListener(u.Host), handler))
	} else {
		log.Printf("Listening on port %v", conf.Port)
		log.Fatal(http.ListenAndServe(":"+conf.Port, handler))
	}
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
			message = fmt.Sprintf("Thank you for signing up. You'll receive your first newsletter at <strong>%s</strong> the next time an edition of <em>%s</em> is published.", res.Email, conf.newsletterName)
		}

		return renderTemplate(w, conf.AssetsDir+"/views/ok", getLocals(map[string]interface{}{
			"message": message,
		}))
	})
}

func handleShow(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		return renderTemplate(w, conf.AssetsDir+"/views/show", getLocals(map[string]interface{}{}))
	})
}

func handleShowConfirmMessagePreview(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		return renderTemplate(w, conf.AssetsDir+"/views/messages/confirm", getLocals(map[string]interface{}{
			"token": "bc492bd9-2aea-458a-aea1-cd7861c334d1",
		}))
	})
}

func handleShowConfirmMessagePlainPreview(w http.ResponseWriter, r *http.Request) {
	withErrorHandling(w, func() error {
		return renderTemplate(w, conf.AssetsDir+"/views/messages/confirm_plain", getLocals(map[string]interface{}{
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

		if res.ConfirmationRateLimited {
			message = fmt.Sprintf("Thank you for signing up. I recently sent a confirmation email to <strong>%s</strong> and don't want to send another one so soon after. Please try to find the message and click the enclosed link to finish signing up for <em>%s</em>. If you can't find it, try checking your spam folder.", email, conf.newsletterName)
		} else {
			message = fmt.Sprintf("Thank you for signing up! I've sent a confirmation email to <strong>%s</strong>. Please click the enclosed link to finish signing up for <em>%s</em>.", email, conf.newsletterName)
		}

		return renderTemplate(w, conf.AssetsDir+"/views/ok", getLocals(map[string]interface{}{
			"message": message,
		}))
	})
}

//
// Private functions
//

func getRateLimiter() (*throttled.HTTPRateLimiter, error) {
	// We use a memory store instead of something like Redis because for the
	// time being we know that this app will only ever run on a single dyno. If
	// that invariant ever changes, the decision should be revisited.
	//
	// All state is lost when the dyno goes to sleep, but since we're using
	// small time scales anyway, that's fine.
	//
	// Note the argument here is the maximum number of allowed keys. Dynos are
	// relatively large, so pick a number big enough to give us a lot of
	// leeway.
	store, err := memstore.New(65536)
	if err != nil {
		return nil, err
	}

	// Start at 3 allowed tokens and refill at a rate of 3 per second.
	quota := throttled.RateQuota{
		MaxBurst: 3,
		MaxRate:  throttled.PerSec(3),
	}

	rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
	if err != nil {
		return nil, err
	}

	// Vary based off of remote IP.
	return &throttled.HTTPRateLimiter{
		RateLimiter: rateLimiter,
		VaryBy:      &throttled.VaryBy{RemoteAddr: true},
	}, nil
}

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

	err := renderTemplate(w, conf.AssetsDir+"/views/error", getLocals(map[string]interface{}{
		"error": renderErr.Error(),
	}))
	if err != nil {
		// Hopefully it never comes to this
		log.Printf("Error during error handling: %v", err)
	}
}

// serveHTTPSRedirect listens on port 80 and redirects any requests on it to
// HTTPS. This is only used in the case where Let's Encrypt is activated (when
// on Heroku we have a router in front of us and don't need to listen on a
// separate port).
func serveHTTPSRedirect() {
	log.Printf("Listening on port 80 and redirecting to HTTPS")

	redirectHandler := http.NewServeMux()
	redirectHandler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r,
			fmt.Sprintf("https://%s%s", r.Host, r.URL),
			http.StatusPermanentRedirect)
	})
	log.Fatal(http.ListenAndServe(":80", redirectHandler))
}

func withErrorHandling(w http.ResponseWriter, fn func() error) {
	err := fn()
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
}
