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
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/memstore"
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

	// DatabaseMaxOpenConns is the maximum number of open database connections
	// allowed.
	DatabaseMaxOpenConns int `env:"DATABASE_MAX_OPEN_CONNS,default=5"`

	// DatabaseURL is the URL to the Postgres database used to store program
	// state.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// EnableRateLimiter activates rate limiting on source IP to make it more
	// difficult for attackers to burn through resource limits. It is on by
	// default.
	EnableRateLimiter bool `env:"ENABLE_RATE_LIMITER,default=true"`

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
	Port string `env:"PORT,default=5001"`

	// PublicURL is the public location from which the site is being served.
	// This is needed in some places to generate absolute URLs. Also used for
	// CSRF protection.
	PublicURL string `env:"PUBLIC_URL,default=https://passages-signup.herokuapp.com"`

	// Some newsletter-specific properties that are set based off the value of Newsletter.
	listAddress            string
	newsletterName         NewsletterName
	newsletterDescription  string // First paragraph: Shown on web + in Twitter card
	newsletterDescription2 string // Second paragraph: Shown only on web
	newsletterAboutPhoto   string
}

// NewsletterID identifies a newsletter and its values are used as options for
// an incoming environmental variable.
type NewsletterID string

// NewsletterName represents the name of a newsletter.
type NewsletterName string

const (
	nanoglyphID           NewsletterID   = "nanoglyph"
	nanoglyphName         NewsletterName = "Nanoglyph"
	nanoglyphDescription  string         = `<em>` + string(nanoglyphName) + `</em> is a weekly newsletter about software, with a focus on simplicity and sustainability. It usually consists of a few links with editorial. It's written by <a href="https://brandur.org">brandur</a>.`
	nanoglyphDescription2 string         = `Check out a <a href="https://brandur.org/nanoglyphs/006-moma-rain">sample edition</a>. Sign up above to have new ones delivered fresh to your inbox whenever they're published.`
	nanoglyphAboutPhoto   string         = "Background photo is the <em>Blue Planet Sky</em> exhibit at the 21st Century Museum of Contemporary Art in Kanazawa, Japan. (And taken on a day that saw much more grey than blue.)"

	passagesID           NewsletterID   = "passages"
	passagesName         NewsletterName = "Passages & Glass"
	passagesDescription  string         = `<em>` + string(passagesName) + `</em> is a personal newsletter about exploration, ideas, and software written by <a href="https://brandur.org">brandur</a>. It's sent rarely – just a few times a year.`
	passagesDescription2 string         = `Check out a <a href="https://brandur.org/passages/003-koya">sample edition</a>. Sign up above to have new ones sent to you. Easily unsubscribe at any time with a single click.`
	passagesAboutPhoto   string         = "Background photo is a distorted selection of wild California grass. Taken along Mission Creek in San Francisco."
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
		conf.newsletterDescription2 = nanoglyphDescription2
		conf.newsletterAboutPhoto = nanoglyphAboutPhoto

	case passagesID:
		conf.newsletterName = passagesName
		conf.newsletterDescription = passagesDescription
		conf.newsletterDescription2 = passagesDescription2
		conf.newsletterAboutPhoto = passagesAboutPhoto

	default:
		log.Fatalf("Unknown newsletter configuration (`NEWSLETTER_ID`): %s (should be either %s or %s)",
			conf.NewsletterID, nanoglyphID, passagesID)
	}
	conf.listAddress = string(conf.NewsletterID) + "@" + mailDomain

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
	if conf.EnableRateLimiter {
		rateLimiter, err := getRateLimiter()
		if err != nil {
			log.Fatal(err)
		}
		handler = rateLimiter.RateLimit(handler)
	}

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

		db, err := OpenDB()
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
			message = fmt.Sprintf(`<p>You've been signed up successfully.</p><p>You'll receive your first edition of <em>%s</em> at <strong>%s</strong> the next time one is published.</p>`, conf.newsletterName, res.Email)
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

		db, err := OpenDB()
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
			message = fmt.Sprintf("<p>Thank you for signing up!</p><p>I recently sent a confirmation email to <strong>%s</strong> and don't want to send another one so soon after. Please try to find the message and click the enclosed link to finish signing up for <em>%s</em>. If you can't find it, try checking your spam folder.</p>", email, conf.newsletterName)
		} else {
			message = fmt.Sprintf("<p>Thank you for signing up!</p><p>I've sent a confirmation email to <strong>%s</strong>. Please click the enclosed link to finish signing up for <em>%s</em>.</p>", email, conf.newsletterName)
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

	quota := throttled.RateQuota{
		MaxBurst: 30,
		MaxRate:  throttled.PerSec(10),
	}

	rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
	if err != nil {
		return nil, err
	}

	deniedHandler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Rate limit exceeded. Sorry about that -- please try again in a few seconds.", 429)
	}))

	// Vary based off of remote IP.
	return &throttled.HTTPRateLimiter{
		DeniedHandler: deniedHandler,
		RateLimiter:   rateLimiter,
		VaryBy:        &throttled.VaryBy{RemoteAddr: true},
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

func withErrorHandling(w http.ResponseWriter, fn func() error) {
	err := fn()
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
}
