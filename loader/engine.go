package loader

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/object88/options/loader/collections"
	"github.com/object88/options/log"
)

type stateChangeEvent struct {
	l    *Loader
	hash collections.Hash
}

// Engine is a Go code loader
type Engine struct {
	closer chan bool

	caravan  *collections.Caravan
	packages map[string]*Package

	stateChange chan *stateChangeEvent

	Log log.Logger
}

// NewEngine creates a new loader
func NewEngine(l log.Logger) *Engine {
	le := &Engine{
		caravan:     collections.CreateCaravan(),
		closer:      make(chan bool),
		Log:         l,
		packages:    map[string]*Package{},
		stateChange: make(chan *stateChangeEvent),
	}

	go func() {
		stop := false
		for !stop {
			select {
			case <-le.closer:
				stop = true
			case e := <-le.stateChange:
				go le.processStateChange(e)
			}
		}

		le.Log.Debugf("Start: ending anon go func\n")
	}()

	return le
}

// Close stops the loader engine processing
func (le *Engine) Close() error {
	le.closer <- true
	return nil
}

func (le *Engine) readDir(l *Loader, absPath string) {
	if !l.isAllowed(absPath) {
		le.Log.Verbosef("readDir: directory '%s' is not allowed\n", absPath)
		return
	}

	le.Log.Debugf("readDir: queueing '%s'...\n", absPath)

	le.ensurePackage(l, absPath)

	fis, err := l.context.ReadDir(absPath)
	if err != nil {
		panic(fmt.Sprintf("Dang:\n\t%s", err.Error()))
	}
	for _, fi := range fis {
		// Ignore individual files
		if !fi.IsDir() {
			continue
		}

		if fi.Name() == "vendor" {
			continue
		}

		le.readDir(l, filepath.Join(absPath, fi.Name()))
	}
}

func (le *Engine) processStateChange(sce *stateChangeEvent) {
	n, _ := le.caravan.Find(sce.hash)
	p := n.Element.(*Package)

	loadState := p.loadState.get()
	le.Log.Debugf("PSC: %s: current state: %d\n", p, loadState)

	switch loadState {
	case queued:
		le.processDirectory(sce.l, p)

		p.loadState.increment()
		p.c.Broadcast()
		le.stateChange <- sce
	case unloaded:
		importPaths := map[string]bool{}
		haveGo := le.processGoFiles(sce.l, p, importPaths)
		haveCgo := le.processCgoFiles(sce.l, p, importPaths)
		if haveGo || haveCgo {
			imports := importPathMapToArray(importPaths)
			le.processPackages(sce.l, p, imports, false)
			le.processComplete(sce.l, p)
		}

		p.loadState.increment()
		p.c.Broadcast()
		le.stateChange <- sce
	case done:
		if sce.l.areAllPackagesComplete() {
			le.Log.Debugf("All packages are loaded\n")
			sce.l.Signal()
		}
	}
}

func importPathMapToArray(imports map[string]bool) []string {
	i := 0
	results := make([]string, len(imports))
	for k := range imports {
		results[i] = k
		i++
	}
	return results
}

func (le *Engine) processComplete(l *Loader, p *Package) {
	if l.isUnsafe(p) {
		le.Log.Debugf(" PC: %s: Checking unsafe (skipping)\n", p)
		return
	}

	err := l.checkPackage(p)
	if err != nil {
		le.Log.Debugf(" PC: %s: Error while checking %s:\n\t%s\n\n", p, p.AbsPath, err.Error())
	}
}

func (le *Engine) processDirectory(l *Loader, p *Package) {
	if l.isUnsafe(p) {
		le.Log.Debugf("*** Loading `%s`, replacing with types.Unsafe\n", p)
		p.typesPkg = types.Unsafe

		le.caravan.Insert(p)
	} else {
		err := p.generateBuildPackage()
		if err != nil {
			le.Log.Debugf("importBuildPackage: %s\n", err.Error())
		}
	}
}

func (le *Engine) processGoFiles(l *Loader, p *Package, importPaths map[string]bool) bool {
	if l.isUnsafe(p) || p.buildPkg == nil {
		return false
	}

	fnames := p.buildPkg.GoFiles
	if len(fnames) == 0 {
		return false
	}

	for _, fname := range fnames {
		fpath := filepath.Join(p.AbsPath, fname)
		le.processFile(l, p, fname, fpath, fpath, importPaths)
	}

	return true
}

func (le *Engine) processCgoFiles(l *Loader, p *Package, importPaths map[string]bool) bool {
	if l.isUnsafe(p) || p.buildPkg == nil {
		return false
	}

	fnames := p.buildPkg.CgoFiles
	if len(fnames) == 0 {
		return false
	}

	cgoCPPFLAGS, _, _, _ := cflags(p.buildPkg, true)
	_, cgoexeCFLAGS, _, _ := cflags(p.buildPkg, false)

	if len(p.buildPkg.CgoPkgConfig) > 0 {
		pcCFLAGS, err := pkgConfigFlags(p.buildPkg)
		if err != nil {
			le.Log.Debugf("CGO: %s: Failed to get flags: %s\n", p, err.Error())
			return false
		}
		cgoCPPFLAGS = append(cgoCPPFLAGS, pcCFLAGS...)
	}

	fpaths := make([]string, len(fnames))
	for k, v := range fnames {
		fpaths[k] = filepath.Join(p.AbsPath, v)
	}

	tmpdir, _ := ioutil.TempDir("", strings.Replace(p.AbsPath, "/", "_", -1)+"_C")
	var files, displayFiles []string

	// _cgo_gotypes.go (displayed "C") contains the type definitions.
	files = append(files, filepath.Join(tmpdir, "_cgo_gotypes.go"))
	displayFiles = append(displayFiles, "C")
	for _, fname := range fnames {
		// "foo.cgo1.go" (displayed "foo.go") is the processed Go source.
		f := cgoRe.ReplaceAllString(fname[:len(fname)-len("go")], "_")
		files = append(files, filepath.Join(tmpdir, f+"cgo1.go"))
		displayFiles = append(displayFiles, fname)
	}

	var cgoflags []string
	if p.buildPkg.Goroot && p.buildPkg.ImportPath == "runtime/cgo" {
		cgoflags = append(cgoflags, "-import_runtime_cgo=false")
	}
	if p.buildPkg.Goroot && p.buildPkg.ImportPath == "runtime/race" || p.buildPkg.ImportPath == "runtime/cgo" {
		cgoflags = append(cgoflags, "-import_syscall=false")
	}

	args := []string{
		"tool",
		"cgo",
		"-objdir",
		tmpdir,
	}
	for _, f := range cgoflags {
		args = append(args, f)
	}
	args = append(args, "--")
	args = append(args, "-I")
	args = append(args, tmpdir)
	for _, v := range cgoCPPFLAGS {
		args = append(args, v)
	}
	for _, v := range cgoexeCFLAGS {
		args = append(args, v)
	}
	for _, f := range fnames {
		args = append(args, f)
	}

	cmd := exec.Command("go", args...)
	cmd.Dir = p.AbsPath
	cmd.Stdout = os.Stdout // os.Stderr
	cmd.Stderr = os.Stdout // os.Stderr
	if err := cmd.Run(); err != nil {
		le.Log.Debugf("CGO: %s: ERROR: cgo failed: %s\n\t%s\n", p, args, err.Error())
		return false
	}

	for i, fpath := range files {
		fname := filepath.Base(fpath)
		le.processFile(l, p, fname, fpath, displayFiles[i], importPaths)
	}
	le.Log.Debugf("CGO: %s: Done processing\n", p)

	return true
}

func (le *Engine) processFile(l *Loader, p *Package, fname, fpath, displayPath string, importPaths map[string]bool) {
	r, err := l.context.OpenFile(fpath)
	if err != nil {
		le.Log.Debugf("ERROR: Failed to open file %s:\n\t%s\n", fpath, err.Error())
		return
	}

	astf, err := parser.ParseFile(p.Fset, displayPath, r, parser.ParseComments|parser.AllErrors)

	if c, ok := r.(io.Closer); ok {
		c.Close()
	}

	if err != nil {
		le.Log.Debugf("ERROR: While parsing %s:\n\t%s\n", fpath, err.Error())
	}

	if p.astPkg == nil {
		p.astPkg = &ast.Package{
			Name:  astf.Name.Name,
			Files: map[string]*ast.File{displayPath: astf},
		}
	} else {
		p.astPkg.Files[displayPath] = astf
	}

	le.findImportPathsFromAst(astf, importPaths)

	p.files[fname] = astf
}

func (le *Engine) processPackages(l *Loader, p *Package, importPaths []string, testing bool) {
	loadState := p.loadState.get()
	le.Log.Debugf(" PP: %s: %d: started\n", p, loadState)

	importedPackages := map[string]bool{}

	for _, importPath := range importPaths {
		targetPath, err := l.FindImportPath(p, importPath)
		if err != nil {
			le.Log.Debugf(" PP: %s: %d: %s\n\t%s\n", p, loadState, err.Error())
			continue
		}
		le.ensurePackage(l, targetPath)

		importedPackages[targetPath] = true
	}

	p.docPkg = doc.New(p.astPkg, "", doc.AllDecls|doc.AllMethods|doc.PreserveAST)

	// TEMPORARY
	func() {
		imprts := []string{}
		for importedPackage := range importedPackages {
			if targetP, err := l.FindPackage(importedPackage); err != nil {
				continue
			} else {
				imprts = append(imprts, targetP.String())
			}
		}
		allImprts := strings.Join(imprts, ", ")
		le.Log.Debugf(" PP: %s: %d: -> %s\n", p, loadState, allImprts)
	}()

	for importPath := range importedPackages {
		targetP, err := l.FindPackage(importPath)
		if err != nil {
			le.Log.Debugf(" PP: %s: %d: import path is missing: %s\n", p, loadState, importPath)
			continue
		}

		targetP.WaitUntilReady(loadState)

		if testing {
			err = le.caravan.WeakConnect(p, targetP)
		} else {
			err = le.caravan.Connect(p, targetP)
		}

		if err != nil {
			panic(fmt.Sprintf(" PP: %s: %d: [weak] connect failed:\n\tfrom: %s\n\tto: %s\n\terr: %s\n\n", p, loadState, p, targetP, err.Error()))
		}
	}
	// All dependencies are loaded; can proceed.
	le.Log.Debugf(" PP: %s: %d: all imports fulfilled.\n", p, loadState)
}

func (le *Engine) ensurePackage(l *Loader, absPath string) {
	dp, created := l.ensurePackage(absPath)

	if created {
		le.stateChange <- &stateChangeEvent{
			l:    l,
			hash: dp.Hash(),
		}
	}
}

func (le *Engine) findImportPathsFromAst(astf *ast.File, importPaths map[string]bool) {
	for _, decl := range astf.Decls {
		decl, ok := decl.(*ast.GenDecl)
		if !ok || decl.Tok != token.IMPORT {
			continue
		}

		for _, spec := range decl.Specs {
			spec := spec.(*ast.ImportSpec)

			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || path == "C" {
				// Ignore any error and skip the C pseudo package
				continue
			}

			importPaths[path] = true
		}
	}
}
