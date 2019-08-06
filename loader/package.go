package loader

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/token"
	"go/types"
	"sync"

	"github.com/object88/options/loader/collections"
	"github.com/pkg/errors"
)

// Package contains the os/arch specific package AST
type Package struct {
	AbsPath string
	hash    collections.Hash

	Fset *token.FileSet

	l         *Loader
	loadState loadState

	m sync.Mutex
	c *sync.Cond

	astPkg   *ast.Package
	buildPkg *build.Package
	docPkg   *doc.Package
	checker  *types.Checker
	files    map[string]*ast.File
	typesPkg *types.Package
}

// NewPackage returns a new instance of Package
func NewPackage(l *Loader, absPath string) *Package {
	hash := collections.CalculateHashFromString(absPath)

	dp := &Package{
		Fset:    token.NewFileSet(),
		AbsPath: absPath,
		files:   map[string]*ast.File{},
		hash:    hash,
		l:       l,
	}
	dp.c = sync.NewCond(&dp.m)

	return dp
}

func (p *Package) check() error {
	if p.checker == nil {
		info := &types.Info{
			Defs: map[*ast.Ident]types.Object{},
			// Implicits:  map[ast.Node]types.Object{},
			Scopes: map[ast.Node]*types.Scope{},
			// Selections: map[*ast.SelectorExpr]*types.Selection{},
			Types: map[ast.Expr]types.TypeAndValue{},
			// Uses: map[*ast.Ident]types.Object{},
		}

		p.typesPkg = types.NewPackage(p.AbsPath, p.buildPkg.Name)
		p.checker = types.NewChecker(p.l.config, p.Fset, p.typesPkg, info)
	}

	// Loop over files and clear previous errors; all will be rechecked.
	astFiles := make([]*ast.File, len(p.files))
	i := 0
	for _, v := range p.files {
		f := v
		astFiles[i] = f
		i++
	}

	if len(astFiles) == 0 {
		return nil
	}

	p.m.Lock()
	err := p.checker.Files(astFiles)
	p.m.Unlock()

	if err != nil {
		return errors.Wrapf(err, "Package.check (%s): Checker failed", p)
	}

	return nil
}

func (p *Package) generateBuildPackage() error {
	buildPkg, err := p.l.context.Import(".", p.AbsPath, 0)
	if err != nil {
		if _, ok := err.(*build.NoGoError); ok {
			// There isn't any Go code here.
			return nil
		}
		return errors.Wrapf(err, "generateBuildPackage (%s): error while importing with build.Context", p)
	}

	p.buildPkg = buildPkg
	return nil
}

// Hash returns the hash for this package
func (p *Package) Hash() collections.Hash {
	return p.hash
}

func (p *Package) Name() string {
	return p.astPkg.Name
}

func (p *Package) String() string {
	return fmt.Sprintf("%s %s", p.l.getTags(), p.AbsPath)
}

func (p *Package) FindSource(structName string) (string, *types.Struct, error) {
	for k, v := range p.checker.Defs {
		tn, ok := v.(*types.TypeName)
		if !ok {
			continue
		}

		if tn.Name() != structName {
			continue
		}

		n, ok := tn.Type().(*types.Named)
		if !ok {
			return "", nil, errors.Errorf("Found identifier '%s', type '%s' is not a named struct", structName, tn.Type().String())
		}
		s, ok := n.Underlying().(*types.Struct)
		if !ok {
			return "", nil, errors.Errorf("Found identifier '%s', underlying type '%s' is not a struct", structName, n.Underlying().String())
		}

		f := p.Fset.File(k.Pos())
		return f.Name(), s, nil
	}

	return "", nil, errors.Errorf("Failed to locate struct '%s' within package '%s'", structName, p.Name())
}

// WaitUntilReady blocks until this package has loaded sufficiently for the
// requested load state.
func (p *Package) WaitUntilReady(loadState loadState) {
	check := func() bool {
		thisLoadState := p.loadState.get()

		switch loadState {
		case queued:
			// Does not make sense that the source loadState would be here.
		case unloaded:
			return thisLoadState > unloaded
		default:
			// Should never get here.
		}

		return false
	}

	p.m.Lock()
	for !check() {
		p.c.Wait()
	}
	p.m.Unlock()
}
