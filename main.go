package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v4"
	"github.com/joeshaw/envdecode"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/memstore"
	"golang.org/x/xerrors"

	"github.com/brandur/csrf"
	"github.com/brandur/passages-signup/command"
	"github.com/brandur/passages-signup/db"
	"github.com/brandur/passages-signup/mailclient"
	"github.com/brandur/passages-signup/newslettermeta"
	"github.com/brandur/passages-signup/ptemplate"
)

const (
	envProduction = "production"
	envTesting    = "testing"

	mailDomain     = "list.brandur.org"
	replyToAddress = "brandur@brandur.org"
)

var validate = validator.New()

// Conf contains configuration information for the command. It's extracted from
// environment variables.
type Conf struct {
	// DatabaseTXStarter is a special value used to inject a test transaction to
	// the server. Will be used instead of DatabaseURL if specified.
	DatabaseTXStarter db.TXStarter `env:"-" validate:"required_without=DatabaseURL"`

	// DatabaseURL is the URL to the Postgres database used to store program
	// state.
	DatabaseURL string `env:"DATABASE_URL,required" validate:"required_without=DatabaseTXStarter"`

	// EnableRateLimiter activates rate limiting on source IP to make it more
	// difficult for attackers to burn through resource limits. It is on by
	// default.
	EnableRateLimiter bool `env:"ENABLE_RATE_LIMITER,default=true" validate:"-"`

	// MailgunAPIKey is a key for Mailgun used to send email.
	MailgunAPIKey string `env:"MAILGUN_API_KEY,required" validate:"required"`

	// Newsletter is the newsletter to send. Should be either `nanoglyph` or
	// `passages` and defaults to the latter. Along with one of the available
	// values it should also be the identifier of the list in Mailgun.
	NewsletterID string `env:"NEWSLETTER_ID,default=passages" validate:"required"`

	// PassagesEnv determines the running environment of the app. Set to
	// development to disable template caching and CSRF protection.
	PassagesEnv string `env:"PASSAGES_ENV,default=production" validate:"required"`

	// Port is the port over which to serve HTTP.
	Port string `env:"PORT,default=5001" validate:"required"`

	// PublicURL is the public location from which the site is being served.
	// This is needed in some places to generate absolute URLs. Also used for
	// CSRF protection.
	PublicURL string `env:"PUBLIC_URL,default=https://passages-signup.herokuapp.com" validate:"required"`
}

func (c *Conf) isProduction() bool {
	return c.PassagesEnv == envProduction
}

var (
	//go:embed public/*
	embeddedAssets embed.FS

	//go:embed layouts/* views/*
	embeddedTemplates embed.FS
)

type Server struct {
	conf      *Conf
	handler   http.Handler
	mailAPI   mailclient.API
	meta      *newslettermeta.Meta
	renderer  *ptemplate.Renderer
	txStarter db.TXStarter
}

func main() {
	var conf Conf
	err := envdecode.Decode(&conf)
	if err != nil {
		logrus.Fatalf("Error decoding env configuration: %v", err)
	}

	server, err := NewServer(&conf)
	if err != nil {
		logrus.Fatalf("Error initiaizing server: %v", err)
	}

	if err := server.Start(); err != nil {
		logrus.Fatalf("Error starting server: %v", err)
	}
}

func NewServer(conf *Conf) (*Server, error) {
	if err := validate.Struct(conf); err != nil {
		return nil, xerrors.Errorf("error validating server config: %w", conf)
	}

	ctx := context.Background()

	meta, err := newslettermeta.MetaFor(mailDomain, conf.NewsletterID)
	if err != nil {
		return nil, err
	}

	var mailAPI mailclient.API
	if conf.PassagesEnv == envTesting {
		mailAPI = mailclient.NewFakeClient()
	} else {
		mailAPI = mailclient.NewMailgunClient(mailDomain, conf.MailgunAPIKey)
	}

	// Use templates embedded with `go:embed` in production, but local
	// filesystem otherwise so we can easily iterate in development.
	var templates fs.FS
	if conf.isProduction() {
		templates = embeddedTemplates
	} else {
		templates = os.DirFS(".")
	}

	renderer, err := ptemplate.NewRenderer(&ptemplate.RendererConfig{
		DynamicReload:  !conf.isProduction(),
		NewsletterMeta: meta,
		PublicURL:      conf.PublicURL,
		Templates:      templates,
	})
	if err != nil {
		return nil, err
	}

	txStarter := conf.DatabaseTXStarter
	if txStarter == nil {
		txStarter, err = db.Connect(ctx, &db.ConnectConfig{
			ApplicationName: "passages-signup",
			DatabaseURL:     conf.DatabaseURL,
		})
		if err != nil {
			return nil, err
		}
	}

	s := &Server{
		conf:      conf,
		mailAPI:   mailAPI,
		meta:      meta,
		renderer:  renderer,
		txStarter: txStarter,
	}

	r := mux.NewRouter()
	r.HandleFunc("/", s.handleShow)
	r.HandleFunc("/confirm/{token}", s.handleConfirm)
	r.HandleFunc("/submit", s.handleSubmit)

	// Easy message previews for development.
	if !conf.isProduction() {
		r.HandleFunc("/messages/confirm", s.handleShowConfirmMessagePreview)
		r.HandleFunc("/messages/confirm_plain", s.handleShowConfirmMessagePlainPreview)
	}

	// In production serves assets that have been slurped up with go:embed. In
	// other environments, reads directly from disk for reasy reloading.
	r.PathPrefix("/public/").Handler(staticAssetsHandler(conf.isProduction()))

	s.handler = r

	options := []csrf.Option{
		csrf.AllowedOrigin(conf.PublicURL),

		// And also allow the special origin from `brandur.org` which will
		// cross-post to this app.
		csrf.AllowedOrigin("https://brandur.org"),
	}

	if !conf.isProduction() {
		logrus.Infof("Allowing localhost origin for non-production environment")
		options = append(options,
			csrf.AllowedOrigin("http://localhost:"+conf.Port))
	}
	s.handler = csrf.Protect(options...)(s.handler)

	// Use a rate limiter to prevent enumeration of email addresses and so it's
	// harder to maliciously burn through my Mailgun API limit.
	if conf.EnableRateLimiter {
		logrus.Infof("Enabling memory-backed rate limiting")
		rateLimiter, err := getRateLimiter()
		if err != nil {
			logrus.Fatal(err)
		}
		s.handler = rateLimiter.RateLimit(s.handler)
	}

	if conf.isProduction() {
		s.handler = redirectToHTTPS(s.handler)
	}

	return s, nil
}

func (s *Server) Start() error {
	logrus.Infof("Listening on port %v", s.conf.Port)
	if err := http.ListenAndServe(":"+s.conf.Port, s.handler); err != nil {
		return xerrors.Errorf("error listening on port %q: %w", s.conf.Port, err)
	}
	return nil
}

//
// Handlers ---
//

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	s.withErrorHandling(w, func() error {
		vars := mux.Vars(r)
		token := vars["token"]

		var res *command.SignupFinisherResult
		err := db.WithTransaction(r.Context(), s.txStarter, func(ctx context.Context, tx pgx.Tx) error {
			mediator := &command.SignupFinisher{
				ListAddress: s.meta.ListAddress,
				MailAPI:     s.mailAPI,
				Token:       token,
			}

			var err error
			res, err = mediator.Run(r.Context(), tx)
			return err
		})
		if err != nil {
			return xerrors.Errorf("error finishing signup: %w", err)
		}

		var message string
		if res.TokenNotFound {
			w.WriteHeader(http.StatusNotFound)
			message = "We couldn't find that confirmation token."
		} else {
			message = fmt.Sprintf(`<p>You've been signed up successfully.</p><p>You'll receive your first edition of <em>%s</em> at <strong>%s</strong> the next time one is published.</p>`, s.meta.Name, res.Email)
		}

		return s.renderer.RenderTemplate(w, "views/ok", map[string]interface{}{
			"message": message,
		})
	})
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	s.withErrorHandling(w, func() error {
		return s.renderer.RenderTemplate(w, "views/show", map[string]interface{}{})
	})
}

func (s *Server) handleShowConfirmMessagePreview(w http.ResponseWriter, r *http.Request) {
	s.withErrorHandling(w, func() error {
		return s.renderer.RenderTemplate(w, "views/messages/confirm", map[string]interface{}{
			"token": "bc492bd9-2aea-458a-aea1-cd7861c334d1",
		})
	})
}

func (s *Server) handleShowConfirmMessagePlainPreview(w http.ResponseWriter, r *http.Request) {
	s.withErrorHandling(w, func() error {
		return s.renderer.RenderTemplate(w, "views/messages/confirm_plain", map[string]interface{}{
			"token": "bc492bd9-2aea-458a-aea1-cd7861c334d1",
		})
	})
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	s.withErrorHandling(w, func() error {
		// Only accept form POSTs.
		if r.Method != "POST" {
			http.NotFound(w, r)
			return nil
		}

		err := r.ParseForm()
		if err != nil {
			s.renderError(w, http.StatusBadRequest,
				xerrors.Errorf("error parsing form input: %w", err))
			return nil
		}

		email := r.Form.Get("email")
		if email == "" {
			s.renderError(w, http.StatusUnprocessableEntity,
				xerrors.Errorf("expected input parameter email"))
			return nil
		}

		email = strings.TrimSpace(email)

		var res *command.SignupStarterResult
		err = db.WithTransaction(r.Context(), s.txStarter, func(ctx context.Context, tx pgx.Tx) error {
			logrus.Infof("starting mediator ...")

			mediator := &command.SignupStarter{
				Email:          email,
				ListAddress:    s.meta.ListAddress,
				MailAPI:        s.mailAPI,
				Renderer:       s.renderer,
				ReplyToAddress: replyToAddress,
			}

			var err error
			res, err = mediator.Run(r.Context(), tx)
			return err
		})

		var message string
		if err != nil {
			return xerrors.Errorf("error sending confirmation email: %w", err)
		}

		switch {
		case res.ConfirmationRateLimited:
			message = fmt.Sprintf("<p>Thank you for signing up!</p><p>I recently sent a confirmation email to <strong>%s</strong> and don't want to send another one so soon after. Please try to find the message and click the enclosed link to finish signing up for <em>%s</em>. If you can't find it, try checking your spam folder.</p>", email, s.meta.Name)
		case res.MaxNumAttempts:
			message = fmt.Sprintf("<p>Thank you for signing up!</p><p>I've hit the maximum number of confirmation tries for this email address. Please try to find the message and click the enclosed link to finish signing up for <em>%s</em>. If you can't find it, try checking your spam folder.</p>", s.meta.Name)
		default:
			message = fmt.Sprintf("<p>Thank you for signing up!</p><p>I've sent a confirmation email to <strong>%s</strong>. Please click the enclosed link to finish signing up for <em>%s</em>.</p>", email, s.meta.Name)
		}

		return s.renderer.RenderTemplate(w, "views/ok", map[string]interface{}{
			"message": message,
		})
	})
}

//
// Private functions
//

func (s *Server) renderError(w http.ResponseWriter, status int, renderErr error) {
	w.WriteHeader(status)

	err := s.renderer.RenderTemplate(w, "views/error", map[string]interface{}{
		"error": renderErr.Error(),
	})
	if err != nil {
		// Hopefully it never comes to this
		logrus.Infof("Error during error handling: %v", err)
	}
}

func (s *Server) withErrorHandling(w http.ResponseWriter, fn func() error) {
	if err := fn(); err != nil {
		logrus.Errorf("Internal server error: %v", err)
		s.renderError(w, http.StatusInternalServerError, err)
		return
	}
}

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
		return nil, xerrors.Errorf("error initializing memory store: %w", err)
	}

	quota := throttled.RateQuota{
		MaxBurst: 20,
		MaxRate:  throttled.PerSec(5),
	}

	rateLimiter, err := throttled.NewGCRARateLimiter(store, quota)
	if err != nil {
		return nil, xerrors.Errorf("error initializing rate limiter: %w", err)
	}

	deniedHandler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Rate limit exceeded. Sorry about that -- please try again in a few seconds.", http.StatusTooManyRequests)
	}))

	// Vary based off of remote IP.
	return &throttled.HTTPRateLimiter{
		DeniedHandler: deniedHandler,
		RateLimiter:   rateLimiter,
		VaryBy:        &throttled.VaryBy{RemoteAddr: true},
	}, nil
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

func staticAssetsHandler(useEmbedded bool) http.Handler {
	if useEmbedded {
		return http.FileServer(http.FS(embeddedAssets))
	}

	return http.StripPrefix("/public/", http.FileServer(http.Dir("./public")))
}
