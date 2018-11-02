package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/brandur/csrf"
	"github.com/gorilla/mux"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/yosssi/ace"
)

const (
	envProduction = "production"
	envTesting    = "testing"
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
	vars := mux.Vars(r)
	token := vars["token"]

	db, err := sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}

	mailAPI := getMailAPI()

	mediator := &SignupFinisher{
		DB:      db,
		MailAPI: mailAPI,
		Token:   token,
	}
	res, err := mediator.Run()

	template, err := getTemplate("views/submit")
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}

	var message string
	if err != nil {
		err = errors.Wrap(err, "Encountered a problem finishing signup")
		message = err.Error()
	} else if res.TokenNotFound {
		w.WriteHeader(http.StatusNotFound)
		message = "We couldn't find that confirmation token."
	} else {
		message = fmt.Sprintf("Thank you for signing up. You'll receive your first newsletter at <strong>%s</strong> the next time an edition of <em>Passages & Glass</em> is published.", res.Email)
	}

	locals := getLocals(map[string]interface{}{
		"message": message,
	})
	err = template.Execute(w, locals)
	if err != nil {
		// Body may have already been sent, so just respond normally.
		log.Printf("Error rendering template: %v", err)
		return
	}
}

func handleShow(w http.ResponseWriter, r *http.Request) {
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
		return
	}

	email := r.Form.Get("email")
	if email == "" {
		renderError(w, http.StatusUnprocessableEntity,
			fmt.Errorf("Expected input parameter email"))
		return
	}

	email = strings.TrimSpace(email)

	template, err := getTemplate("views/submit")
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}

	db, err := sql.Open("postgres", conf.DatabaseURL)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}

	mailAPI := getMailAPI()

	mediator := &SignupStarter{
		DB:      db,
		Email:   email,
		MailAPI: mailAPI,
	}
	res, err := mediator.Run()

	var message string
	if err != nil {
		err = errors.Wrap(err, "Encountered a problem sending a confirmation email")
		message = err.Error()
	} else if res.ConfirmationRateLimited {
		message = fmt.Sprintf("Thank you for signing up. We recently sent a confirmation email to <strong>%s</strong> and don't want to send another one so soon. Please try to find the message and click the enclosed link to finish signing up for <em>Passages & Glass</em> (check your spam folder if you can't find it).", email)
	} else {
		message = fmt.Sprintf("Thank you for signing up. We've sent a confirmation email to <strong>%s</strong>. Please click the enclosed link to finish signing up for <em>Passages & Glass</em>.", email)
	}

	locals := getLocals(map[string]interface{}{
		"message": message,
	})
	err = template.Execute(w, locals)
	if err != nil {
		// Body may have already been sent, so just respond normally.
		log.Printf("Error rendering template: %v", err)
		return
	}
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

// getLocals injects a default set of local variables that are needed for
// rendering any template and then includes in those specified in the locals
// parameter for this particular run.
func getLocals(locals map[string]interface{}) map[string]interface{} {
	defaults := map[string]interface{}{
		"publicURL": conf.PublicURL,
	}

	for k, v := range locals {
		defaults[k] = v
	}

	return defaults
}

func getTemplate(file string) (*template.Template, error) {
	if conf.PassagesEnv != envProduction {
		ace.FlushCache()
	}
	return ace.Load("layouts/main", file, nil)
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
