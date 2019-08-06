package options

import (
	"reflect"
)

type Optioner interface {
	Apply(options ...Option) error
}

type Option interface {
	TargetType() reflect.Type
	Apply(target interface{}) error
}
