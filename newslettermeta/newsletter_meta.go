package newslettermeta

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"
)

var validate = validator.New()

type Meta struct {
	ID                    string `validate:"required"`
	Name                  string `validate:"required"`
	Description           string `validate:"required"`
	Description2          string `validate:"required"`
	DescriptionAboutPhoto string `validate:"required"`
	ListAddress           string `validate:"-"` // filled later
}

const NanoglyphID = "nanoglyph"

var nanoglyphMeta = Meta{
	ID:                    NanoglyphID,
	Name:                  "Nanoglyph",
	Description:           `<em>Nanoglyph</em> is a weekly newsletter about software, with a focus on simplicity and sustainability. It usually consists of a few links with editorial. It's written by <a href="https://brandur.org">brandur</a>.`,
	Description2:          `Check out a <a href="https://brandur.org/nanoglyphs/006-moma-rain">sample edition</a>. Sign up above to have new ones delivered fresh to your inbox whenever they're published.`,
	DescriptionAboutPhoto: "Background photo is the <em>Blue Planet Sky</em> exhibit at the 21st Century Museum of Contemporary Art in Kanazawa, Japan. (And taken on a day that saw much more grey than blue.)",
}

const PassagesID = "passages"

var passagesMeta = Meta{
	ID:                    PassagesID,
	Name:                  "Passages & Glass",
	Description:           `<em>Passages & Glass</em> is a personal newsletter about exploration, ideas, and software written by <a href="https://brandur.org">brandur</a>. It's sent rarely – just a few times a year.`,
	Description2:          `Check out a <a href="https://brandur.org/passages/003-koya">sample edition</a>. Sign up above to have new ones sent to you. Easily unsubscribe at any time with a single click.`,
	DescriptionAboutPhoto: "Background photo is a distorted selection of wild California grass. Taken along Mission Creek in San Francisco.",
}

var metaMap = map[string]Meta{
	nanoglyphMeta.ID: nanoglyphMeta,
	passagesMeta.ID:  passagesMeta,
}

func init() {
	for id, meta := range metaMap {
		m := meta
		if err := validate.Struct(&m); err != nil {
			panic(fmt.Sprintf("error validating meta for newsletter %q: %v", id, err))
		}
	}
}

// MetaFor returns metadata for the given newsletter.
func MetaFor(mailDomain, name string) (*Meta, error) {
	if meta, ok := metaMap[name]; ok {
		meta.ListAddress = meta.ID + "@" + mailDomain
		return &meta, nil // shallow copy
	}

	return nil, xerrors.Errorf("unknown newsletter: %q", name)
}

func MustMetaFor(mailDomain, name string) *Meta {
	meta, err := MetaFor(mailDomain, name)
	if err != nil {
		panic(err)
	}
	return meta
}
