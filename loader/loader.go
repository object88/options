package loader

import (
	"fmt"
	"go/build"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	"github.com/object88/options/loader/collections"
	"github.com/object88/options/log"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

// Loader is the workspace-specific configuration and context for
// building and type-checking
type Loader struct {
	StartDir string

	filteredPaths []glob.Glob
	Tags          []string

	Log log.Logger

	le *Engine

	fs afero.Fs

	config     *types.Config
	context    build.Context
	unsafePath string

	PackageHashSet map[collections.Hash]bool

	m sync.Mutex
	c *sync.Cond
}

// Option provides a hook for NewLoader to set or modify
// the new loader's build.Context
type Option func(l *Loader)

// NewLoader creates a new Loader
func NewLoader(logger log.Logger, options ...Option) *Loader {
	globs := []glob.Glob{
		glob.MustCompile(filepath.Join("**", ".*")),
		glob.MustCompile(filepath.Join("**", "testdata")),
	}

	l := &Loader{
		StartDir:       "--",
		context:        build.Default,
		filteredPaths:  globs,
		fs:             afero.NewReadOnlyFs(afero.NewOsFs()),
		le:             NewEngine(logger),
		Log:            logger,
		PackageHashSet: map[collections.Hash]bool{},
	}

	l.c = sync.NewCond(&l.m)
	l.context.GOARCH = runtime.GOARCH
	l.context.GOOS = runtime.GOOS
	l.context.GOROOT = runtime.GOROOT()

	for _, opt := range options {
		opt(l)
	}

	l.context.IsDir = func(path string) bool {
		fi, err := l.fs.Stat(path)
		return err == nil && fi.IsDir()
	}
	l.context.OpenFile = func(path string) (io.ReadCloser, error) {
		f, err := l.fs.Open(path)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	l.context.ReadDir = func(dir string) ([]os.FileInfo, error) {
		f, err := l.fs.Open(dir)
		if err != nil {
			return nil, err
		}
		list, err := f.Readdir(-1)
		f.Close()
		if err != nil {
			return nil, err
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })

		return list, nil
	}

	l.config = &types.Config{
		Error:    l.HandleTypeCheckerError,
		Importer: &loaderImporter{l: l},
	}
	l.unsafePath = filepath.Join(runtime.GOROOT(), "src", "unsafe")

	return l
}

func (l *Loader) Close() error {
	for hash := range l.PackageHashSet {
		_, ok := l.le.caravan.Find(hash)
		if !ok {
			continue
		}
	}

	return nil
}

// // Errors exposes problems with code found during compilation on a file-by-file
// // basis.
// func (l *Loader) Errors(handleErrs func(file string, errs []FileError)) {
// 	for hash := range l.PackageHashSet {
// 		n, ok := l.le.caravan.Find(hash)
// 		if !ok {
// 			// TODO: This is probably a poor way of handling this problem.  The error
// 			// will bubble up to the user, who will have no idea what the hash means.
// 			errs := []FileError{
// 				FileError{
// 					Message: fmt.Sprintf("Failed to find node in caravan with hash 0x%016x", hash),
// 					Warning: false,
// 				},
// 			}
// 			handleErrs("", errs)
// 			continue
// 		}

// 		dp := n.Element.(*Package)
// 		for fname, f := range dp.files {
// 			if len(f.errs) != 0 {
// 				handleErrs(filepath.Join(dp.AbsPath, fname), f.errs)
// 			}
// 		}
// 	}
// }

// getTags produces a string detailing the GOROOT, GOARCH, GOOS, and any tags
func (l *Loader) getTags() string {
	var sb strings.Builder
	sb.WriteRune('[')
	sb.WriteString(l.context.GOROOT)
	sb.WriteRune(',')
	sb.WriteString(l.context.GOARCH)
	sb.WriteRune(',')
	sb.WriteString(l.context.GOOS)
	for _, v := range l.Tags {
		sb.WriteRune(',')
		sb.WriteString(v)
	}
	sb.WriteRune(']')
	return sb.String()
}

func (l *Loader) areAllPackagesComplete() bool {
	l.m.Lock()

	if len(l.PackageHashSet) == 0 {
		// NOTE: this is a stopgap to address the problem where a loader context
		// will report that all packages are loaded before any of them have been
		// processed.  If we have a situation where a loader context is reading
		// a directory structure where there are legitimately no packages, this
		// will be a problem.
		fmt.Printf("loader.areAllPackagesComplete (%s): have zero packages\n", l)
		l.m.Unlock()
		return false
	}

	complete := true

	caravan := l.le.caravan
	for chash := range l.PackageHashSet {
		n, ok := caravan.Find(chash)
		if !ok {
			fmt.Printf("loader.areAllPackagesComplete (%s): package hash 0x%016x not found in caravan\n", l, chash)
			complete = false
			break
		}
		dp := n.Element.(*Package)
		if !ok {
			fmt.Printf("loader.areAllPackagesComplete (%s): distinct package for %s not found\n", l, dp)
			complete = false
			break
		}
		loadState := dp.loadState.get()
		if loadState != done {
			complete = false
			break
		}
	}

	l.m.Unlock()
	return complete
}

func (l *Loader) checkPackage(p *Package) error {
	l.m.Lock()
	err := p.check()
	l.m.Unlock()
	return err
}

func (l *Loader) ensurePackage(absPath string) (*Package, bool) {
	chash := collections.CalculateHashFromString(absPath)
	n, created := l.le.caravan.Ensure(chash, func() collections.Hasher {
		l.Log.Debugf("ensurePackage: miss on hash 0x%016x; creating package for '%s'.\n", chash, absPath)
		return NewPackage(l, absPath)
	})
	p := n.Element.(*Package)

	l.m.Lock()
	l.PackageHashSet[chash] = true
	l.m.Unlock()

	return p, created
}

// FindPackage will locate the package at the provided path
func (l *Loader) FindPackage(absPath string) (*Package, error) {
	chash := collections.CalculateHashFromString(absPath)
	n, ok := l.le.caravan.Find(chash)
	if !ok {
		return nil, errors.Errorf("Loader does not have an entry for %s with tags %s", absPath, l.getTags())
	}
	p := n.Element.(*Package)
	return p, nil
}

// FindImportPath is used by the loader engine locate the path to a packge
// imported by the specified package
func (l *Loader) FindImportPath(p *Package, importPath string) (string, error) {
	targetPath, err := l.findImportPath(importPath, p.AbsPath)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("Failed to find import %s", importPath))
		return "", err
	}
	if targetPath == p.AbsPath {
		l.Log.Debugf("Failed due to self-import\n")
		return "", err
	}

	return targetPath, nil
}

// LoadDirectory adds the contents of a directory to the Loader
func (l *Loader) LoadDirectory(startDir string) error {
	// if strings.HasPrefix(startDir, "file://") {
	// 	startDir = startDir[utf8.RuneCountInString("file://"):]
	// }
	startDir, err := filepath.Abs(startDir)
	if err != nil {
		return errors.Wrapf(err, "Could not get absolute path for '%s'", startDir)
	}

	if !l.context.IsDir(startDir) {
		return fmt.Errorf("Argument '%s' is not a directory", startDir)
	}

	l.StartDir = startDir
	l.Log.Verbosef("Loader.LoadDirectory: reading dir '%s'\n", l.StartDir)
	l.le.readDir(l, l.StartDir)

	return nil
}

func (l *Loader) isAllowed(absPath string) bool {
	for _, g := range l.filteredPaths {
		if g.Match(absPath) {
			// We are looking at a filtered out path.
			return false
		}
	}

	return true
}

// isUnsafe returns whether the provided package represents the `unsafe`
// package for the loader context
func (l *Loader) isUnsafe(dp *Package) bool {
	return l.unsafePath == dp.AbsPath
}

func (l *Loader) Signal() {
	l.c.Broadcast()
}

// Wait blocks until all packages have been loaded
func (l *Loader) Wait() {
	if l.areAllPackagesComplete() {
		return
	}
	l.c.L.Lock()
	l.c.Wait()
	l.c.L.Unlock()
}

// String is the implementation of fmt.Stringer
func (l *Loader) String() string {
	return fmt.Sprintf("%s %s", l.StartDir, l.getTags())
}

// HandleTypeCheckerError is invoked from the types.Checker when it encounters
// errors
func (l *Loader) HandleTypeCheckerError(e error) {
	if terror, ok := e.(types.Error); ok {
		position := terror.Fset.Position(terror.Pos)
		absPath := filepath.Dir(position.Filename)
		dp, err := l.FindPackage(absPath)
		if err != nil {
			l.Log.Debugf("ERROR: (missing) No package for %s\n\t%s\n", absPath, err.Error())
			return
		}

		baseFilename := filepath.Base(position.Filename)
		_, ok := dp.files[baseFilename]
		if !ok {
			l.Log.Debugf("ERROR: (missing file) %s\n", position.Filename)
		} else {
			l.Log.Debugf("ERROR: (types error) %s\n", terror.Error())
		}
	} else {
		l.Log.Debugf("ERROR: (unknown) %#v\n", e)
	}
}

func (l *Loader) findImportPath(path, src string) (string, error) {
	buildPkg, err := l.context.Import(path, src, build.FindOnly)
	if err != nil {
		msg := fmt.Sprintf("Failed to find import path:\n\tAttempted build.Import('%s', '%s', build.FindOnly)", path, src)
		return "", errors.Wrap(err, msg)
	}
	return buildPkg.Dir, nil
}
