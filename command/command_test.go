package command

import (
	"os"

	"github.com/brandur/passages-signup/newslettermeta"
	"github.com/brandur/passages-signup/ptemplate"
)

const (
	testReplyToAddress = "passages@example.com"
	testListAddress    = "passages@example.com"
)

var renderer *ptemplate.Renderer

func init() {
	var err error
	renderer, err = ptemplate.NewRenderer(&ptemplate.RendererConfig{
		DynamicReload:  true,
		NewsletterMeta: newslettermeta.MustMetaFor("list.brandur.org", newslettermeta.PassagesID),
		PublicURL:      "https://passages.example.com",
		Templates:      os.DirFS(".."),
	})
	if err != nil {
		panic(err)
	}
}
