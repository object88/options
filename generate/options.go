package generate

import "github.com/object88/options/log"

type Option func(g *Generator) error

func SetLog(l log.Logger) Option {
	return func(g *Generator) error {
		g.logger = l
		return nil
	}
}

func SetDestination(d string) Option {
	return func(g *Generator) error {
		return nil
	}
}
