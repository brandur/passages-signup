package main

import (
	"io"
	"log"

	"github.com/pkg/errors"
	"github.com/yosssi/ace"
)

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

// Shortcut for rendering a template and doing the right associated error
// handling.
func renderTemplate(w io.Writer, file string, locals map[string]interface{}) error {
	if conf.PassagesEnv != envProduction {
		ace.FlushCache()
	}

	template, err := ace.Load("layouts/main", file, nil)
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
