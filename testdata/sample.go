package baz

import (
	"errors"
	"reflect"

	"github.com/object88/options"
)

// FooOptions is an option of foo
// generate-options
type FooOptions struct {
	a string
}

func (fo *FooOptions) SetA(a string) options.Option {
	foo := FooOptionsOpt{
		F: func(fo *FooOptions) error {
			fo.a = a
			return nil
		},
	}
	return &foo
}

// -----

type BarFooOptions struct {
	FooOptions
	b string
}

func (bfo *BarFooOptions) SetB(b string) options.Option {
	bfoo := BarFooOptionsOpt{
		F: func(bfo *BarFooOptions) error {
			bfo.b = b
			return nil
		},
	}
	return &bfoo
}

// -----

func (fo *FooOptions) Apply(options ...options.Option) error {
	for _, opt := range options {
		if err := opt.Apply(fo); err != nil {
			return err
		}
	}
	return nil
}

type FooOptionsOpt struct {
	F func(fo *FooOptions) error
}

func (foo *FooOptionsOpt) TargetType() reflect.Type {
	return reflect.TypeOf(FooOptions{})
}

func (foo *FooOptionsOpt) Apply(target interface{}) error {
	mo, ok := target.(*FooOptions)
	if !ok {
		return errors.New("Target is not *FooOptions")
	}
	return foo.F(mo)
}

// -----

func (bfo *BarFooOptions) Apply(optionFuncs ...options.Option) error {
	for _, opt := range optionFuncs {
		if reflect.TypeOf(BarFooOptions{}) == opt.TargetType() {
			if err := opt.Apply(bfo); err != nil {
				return err
			}
		} else {
			sf, ok := reflect.TypeOf(*bfo).FieldByName(opt.TargetType().Name())
			if !ok {
				return errors.New("Missing")
			}
			m, ok := reflect.PtrTo(sf.Type).MethodByName("Apply")
			if !ok {
				return errors.New("Missing apply")
			}
			m.Func.CallSlice(
				[]reflect.Value{
					reflect.ValueOf(bfo).Elem().FieldByName(opt.TargetType().Name()).Addr(),
					reflect.ValueOf([]options.Option{opt}),
				})
		}
	}
	return nil
}
type BarFooOptionsOpt struct {
	F func(fo *BarFooOptions) error
}

func (bfoo *BarFooOptionsOpt) TargetType() reflect.Type {
	return reflect.TypeOf(BarFooOptions{})
}

func (bfoo *BarFooOptionsOpt) Apply(target interface{}) error {
	bfo, ok := target.(*BarFooOptions)
	if !ok {
		return errors.New("Target is not *FooOptions")
	}
	return bfoo.F(bfo)
}
