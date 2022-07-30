package ptemplate

import (
	"html/template"
	"io"
	"io/fs"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
	"github.com/yosssi/ace"
	"golang.org/x/xerrors"

	"github.com/brandur/passages-signup/newslettermeta"
)

var validate = validator.New()

type RendererConfig struct {
	DynamicReload  bool                 `validate:"-"`
	NewsletterMeta *newslettermeta.Meta `validate:"required"`
	PublicURL      string               `validate:"required"`
	Templates      fs.FS                `validate:"required"`
}

type Renderer struct {
	*RendererConfig
	layoutPath string
}

func NewRenderer(config *RendererConfig) (*Renderer, error) {
	if err := validate.Struct(config); err != nil {
		return nil, xerrors.Errorf("error validating renderer config: %w", config)
	}
	return &Renderer{config, "layouts/" + config.NewsletterMeta.ID}, nil
}

// Shortcut for rendering a template and doing the right associated error
// handling.
func (r *Renderer) RenderTemplate(w io.Writer, templateFile string, locals map[string]interface{}) error {
	if strings.HasPrefix(templateFile, "/") {
		return xerrors.Errorf("template file should not start with %q: %q", "/", templateFile)
	}

	locals = r.getLocals(locals)

	logrus.Infof("Rendering: %s [layout: %s]", r.layoutPath, templateFile)

	template, err := ace.Load(r.layoutPath, templateFile, &ace.Options{
		Asset: func(name string) ([]byte, error) {
			f, err := r.Templates.Open(name)
			if err != nil {
				return nil, xerrors.Errorf("error opening template file %q: %w", name, err)
			}
			b, err := io.ReadAll(f)
			if err != nil {
				return nil, xerrors.Errorf("error reading template file %q: %w", name, err)
			}
			return b, nil
		},
		DynamicReload: r.DynamicReload,
		FuncMap: template.FuncMap{
			"StripHTML": stripHTML,
		},
	})
	if err != nil {
		return xerrors.Errorf("error compiling template: %w", err)
	}

	err = template.Execute(w, locals)
	if err != nil {
		err = xerrors.Errorf("error rendering template: %w", err)

		// Body may have already been sent, so just respond normally.
		logrus.Infof("Error: %v", err)
		return nil
	}

	return nil
}

// getLocals injects a default set of local variables that are needed for
// rendering any template and then includes in those specified in the locals
// parameter for this particular run.
func (r *Renderer) getLocals(locals map[string]interface{}) map[string]interface{} {
	defaults := map[string]interface{}{
		"NewsletterMeta": r.NewsletterMeta,
		"PublicURL":      r.PublicURL,
	}

	for k, v := range locals {
		defaults[k] = v
	}

	return defaults
}

var stripHTMLRE = regexp.MustCompile(`<[^>]*>`)

// stripHTML does an extremely basic replacement of all HTML tags with empty
// strings. Not suitable for use with user input.
func stripHTML(content string) string {
	return strings.TrimSpace(stripHTMLRE.ReplaceAllString(content, ""))
}
