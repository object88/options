package generate

import (
	"io"
	"path/filepath"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/object88/options/assets"
	"github.com/object88/options/loader"
	"github.com/object88/options/log"
	"github.com/object88/options/templates"
	"github.com/pkg/errors"
	// "github.com/spf13/afero"
)

// Generator will create options helper structs and functions
type Generator struct {
	logger log.Logger
	// ready  chan struct{}

	l    *loader.Loader
	args *[]Arg
}

type Arg struct {
	Source     string
	StructName string
	// Destination string
}

// NewGenerator creates a new `Generator` instance and returns a pointer to it
func NewGenerator(l *loader.Loader, options ...Option) *Generator {
	g := &Generator{
		logger: log.Stderr(),
		// ready:  make(chan struct{}, 1),
		l: l,
	}

	g.logger.SetLevel(log.Debug)

	for _, opt := range options {
		err := opt(g)
		if err != nil {
			// Crap.
		}
	}

	return g
}

// Generate creates the options source
func (g *Generator) Generate(arg Arg, writer io.Writer) error {
	absFile, err := filepath.Abs(arg.Source)
	if err != nil {
		return err
	}
	p, err := g.l.FindPackage(absFile)
	if err != nil {
		return errors.Wrapf(err, "Failed to find package at '%s'", absFile)
	}

	buf, err := assets.Asset("options.template")
	if err != nil {
		return errors.Wrapf(err, "Failed to get template from assets")
	}
	tmpl, err := template.New("t").Parse(string(buf))
	if err != nil {
		return errors.Wrapf(err, "Failed to load template")
	}

	// fs := afero.NewMemMapFs()
	// afs := &afero.Afero{Fs: fs}
	// f, err = afs.TempFile("", "ioutil-test")
	// if err != nil {
	// 	return errors.Wrapf(err, "Failed to create temp file")
	// }

	_, s, err := p.FindSource(arg.StructName)
	if err != nil {
		return errors.Wrapf(err, "Package '%s' does not contain struct '%s'", p.Name(), arg.StructName)
	}

	data := &templates.Data{
		Now:           time.Now().Format("2006-01-02 15:04:05.000 MST"),
		Package:       p.Name(),
		InstanceName:  createInstanceName(arg.StructName),
		StructName:    arg.StructName,
		StructMembers: make([]templates.FuncData, s.NumFields()),
	}

	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)
		name := f.Name()
		abbreviatedName, capitalizedName := abbreviate(name)
		data.StructMembers[i] = templates.FuncData{
			OptionName:      name,
			OptionNameLower: abbreviatedName,
			OptionNameUpper: capitalizedName,
			OptionType:      f.Type().String(),
		}
	}

	err = tmpl.Execute(writer, data)
	if err != nil {
		return errors.Wrapf(err, "Failed to execute template")
	}

	return nil
}

func createInstanceName(in string) string {
	var name strings.Builder

	r, size := utf8.DecodeRuneInString(in)
	name.WriteRune(unicode.ToLower(r))
	in = in[size:]

	for len(in) > 0 {
		r, size := utf8.DecodeRuneInString(in)
		if unicode.IsUpper(r) {
			name.WriteRune(unicode.ToLower(r))
		}
		in = in[size:]
	}

	return name.String()
}

func abbreviate(in string) (string, string) {
	var abbr strings.Builder
	var cap strings.Builder

	r, size := utf8.DecodeRuneInString(in)
	abbr.WriteRune(r)
	cap.WriteRune(unicode.ToUpper(r))
	in = in[size:]

	for len(in) > 0 {
		r, size := utf8.DecodeRuneInString(in)
		if unicode.IsUpper(r) {
			abbr.WriteRune(unicode.ToLower(r))
		}
		cap.WriteRune(r)
		in = in[size:]
	}

	return abbr.String(), cap.String()
}
