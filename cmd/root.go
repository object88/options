package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/object88/options/generate"
	"github.com/object88/options/loader"
	"github.com/object88/options/log"
	"github.com/spf13/cobra"
)

// InitializeCommands sets up the cobra commands
func InitializeCommands() *cobra.Command {
	rootCmd := createRootCommand()

	return rootCmd
}

type rootCommand struct {
	cobra.Command

	destination string
	packag      string
}

func createRootCommand() *cobra.Command {
	currentDirectory, err := os.Getwd()
	if err != nil {
		// Unknown why this could possibly happen.
		return nil
	}

	var rc *rootCommand
	rc = &rootCommand{
		Command: cobra.Command{
			Use:   "options",
			Short: "options creates embeddable options structs",
			PreRunE: func(cmd *cobra.Command, args []string) error {
				return rc.preexecute(cmd, args)
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				return rc.execute(cmd, args)
			},
		},
	}

	flags := rc.Command.Flags()

	flags.StringVarP(&rc.destination, destinationKey, string(destinationKey[0]), currentDirectory, "Destination for generated options")
	flags.StringVar(&rc.packag, packageKey, string(packageKey[0]), "Package for generated options, defaults to same as destination directory")

	return &rc.Command
}

func (rc *rootCommand) preexecute(cmd *cobra.Command, args []string) error {
	return nil
}

func (rc *rootCommand) execute(cmd *cobra.Command, args []string) error {
	l := loader.NewLoader(log.Stdout())
	g := generate.NewGenerator(l)

	parsedArgs := make([]generate.Arg, len(args))
	for k, arg := range args {
		subs := strings.Split(arg, ":")
		parsedArgs[k].Source = subs[0]
		parsedArgs[k].StructName = subs[1]
	}

	for _, parsedArg := range parsedArgs {
		absPath, err := filepath.Abs(parsedArg.Source)
		if err != nil {
			return err
		}
		l.LoadDirectory(absPath)

		l.Wait()

		p, err := l.FindPackage(absPath)
		if err != nil {
			return err
		}
		filename, _, err := p.FindSource(parsedArg.StructName)
		if err != nil {
			return err
		}

		filename = filepath.Base(filename)
		filename = strings.TrimSuffix(filename, filepath.Ext(filename))
		absPath = path.Join(absPath, fmt.Sprintf("%s_gen.go", filename))
		f, err := os.Create(absPath)
		if err != nil {
			return err
		}
		err = g.Generate(parsedArg, f)
		if err != nil {
			return err
		}
	}

	return nil
}
