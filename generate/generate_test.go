package generate

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/google/uuid"
	"github.com/object88/options/loader"
	logtest "github.com/object88/options/log/testing"
)

type gentest struct {
	name    string
	sources map[string]string
	funcs   map[string]string
}

func Test_Generate(t *testing.T) {
	tcs := []struct {
		name string
		gt   gentest
	}{
		{
			name: "One field",
			gt: gentest{
				sources: map[string]string{
					"fooOptions.go": "package foo\n\ntype FooOptions struct {\n  a string\n}\n",
				},
				funcs: map[string]string{
					"SetA": "string",
				},
			},
		},
		{
			name: "Two fields",
			gt: gentest{
				sources: map[string]string{
					"fooOptions.go": "package foo\n\ntype FooOptions struct {\n  a string\n  b int\n}\n",
				},
				funcs: map[string]string{
					"SetA": "string",
					"SetB": "int",
				},
			},
		},
		{
			name: "Field name variations",
			gt: gentest{
				sources: map[string]string{
					"fooOptions.go": "package foo\n\ntype FooOptions struct {\n  A string\n  b int\n  cWithCamelCase bool}\n",
				},
				funcs: map[string]string{
					"SetA":              "string",
					"SetB":              "int",
					"SetCWithCamelCase": "bool",
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			basepath := writeSource(t, tc.gt.sources)

			l := loadSource(t, basepath)

			g := NewGenerator(l, SetLog(l.Log))

			parsedArg := Arg{
				Source:     basepath,
				StructName: "FooOptions",
			}

			var buf bytes.Buffer
			err := g.Generate(parsedArg, &buf)
			if err != nil {
				t.Errorf("Unexpected error from Generate: %s", err.Error())
			}

			t.Logf("Generated file:\n%s\n", buf.String())

			astf := loadGeneratedCode(t, buf.Bytes())
			evalulateGeneratedCode(t, astf, tc.gt.funcs)
		})
	}
}

func writeSource(t *testing.T, sources map[string]string) string {
	d, err := ioutil.TempDir("", uuid.New().String())
	if err != nil {
		t.Fatalf("Failed to set up test; did not create intermediate temporary dir: %s", err.Error())
	}

	basepath := path.Join(d, "foo")
	for filename, source := range sources {
		fullpath := path.Join(basepath, filename)

		err = os.MkdirAll(path.Dir(fullpath), os.ModeDir|os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to set up test; did not create temporary dir: %s", err.Error())
		}

		f, err := os.Create(fullpath)
		if err != nil {
			pe, ok := err.(*os.PathError)
			if ok {
				t.Fatalf("Failed to set up test; did not create temporary file: op '%s', path '%s', error '%s'", pe.Op, pe.Path, pe.Err.Error())
			}
			t.Fatalf("Failed to set up test; did not create temporary file: %s", err.Error())
		}
		_, err = f.WriteString(source)
		if err != nil {
			t.Fatalf("Failed to set up test; did not write to temporary file: %s", err.Error())
		}
	}

	return basepath
}

func loadSource(t *testing.T, basepath string) *loader.Loader {
	l := loader.NewLoader(logtest.NewLog(t))

	err := l.LoadDirectory(basepath)
	if err != nil {
		t.Errorf("Unexpected error from Load: %s", err.Error())
	}

	l.Wait()

	return l
}

func loadGeneratedCode(t *testing.T, buf []byte) *ast.File {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "src.go", buf, 0)
	if err != nil {
		t.Fatalf("Error while loading generated file")
	}

	if f == nil {
		t.Fatal("Got nil *ast.File reference from load")
	}

	return f
}

func evalulateGeneratedCode(t *testing.T, astf *ast.File, funcs map[string]string) {
	found := map[string]bool{}

	ast.Inspect(astf, func(n ast.Node) bool {
		if n == nil {
			// Nothing here
			return false
		}

		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			// Move on to the next one
			return true
		}

		_, ok = funcs[funcDecl.Name.Name]
		if !ok {
			return false
		}

		if funcDecl.Type.Params.NumFields() != 1 {
			t.Errorf("Found func '%s', has %d parameters", funcDecl.Name.Name, funcDecl.Type.Params.NumFields())
			return false
		}

		// param := funcDecl.Type.Params.List[0]
		// param.Type

		if funcDecl.Type.Results.NumFields() != 1 {
			t.Errorf("Found func '%s', has %d return parameters", funcDecl.Name.Name, funcDecl.Type.Results.NumFields())
			return false
		}

		retType := funcDecl.Type.Results.List[0]
		t.Logf("Return type '%#v'\n", retType.Type)

		if _, ok := found[funcDecl.Name.Name]; ok {
			t.Errorf("Found func '%s' more than once", funcDecl.Name.Name)
			return false
		}
		found[funcDecl.Name.Name] = true
		return true
	})

	if len(found) != len(funcs) {
		t.Error("Mismatch number of funcs")
	}
}
