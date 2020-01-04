package main

import (
	"html/template"
	"io"
	"log"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/yosssi/ace"
)

// getLocals injects a default set of local variables that are needed for
// rendering any template and then includes in those specified in the locals
// parameter for this particular run.
func getLocals(locals map[string]interface{}) map[string]interface{} {
	defaults := map[string]interface{}{
		"newsletterDescription": string(conf.newsletterDescription),
		"newsletterName":        string(conf.newsletterName),
		"publicURL":             conf.PublicURL,
	}

	for k, v := range locals {
		defaults[k] = v
	}

	return defaults
}

// Shortcut for rendering a template and doing the right associated error
// handling.
func renderTemplate(w io.Writer, file string, locals map[string]interface{}) error {
	if conf.PassagesEnv != envProduction {
		ace.FlushCache()
	}

	template, err := ace.Load(conf.AssetsDir+"/layouts/main", file, &ace.Options{
		FuncMap: template.FuncMap{
			"StripHTML": stripHTML,
		},
	})
	if err != nil {
		return errors.Wrap(err, "Failed to compile template")
	}

	err = template.Execute(w, locals)
	if err != nil {
		err = errors.Wrap(err, "Failed to render template")

		// Body may have already been sent, so just respond normally.
		log.Printf("Error: %v", err)
		return nil
	}

	return nil
}

var stripHTMLRE = regexp.MustCompile(`<[^>]*>`)

// stripHTML does an extremely basic replacement of all HTML tags with empty
// strings. Not suitable for use with user input.
func stripHTML(content string) string {
	return strings.TrimSpace(stripHTMLRE.ReplaceAllString(content, ""))
}
