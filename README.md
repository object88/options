# options

## Use Case

You have a constructor-type func which accepts optional funcs as parameters.  Rob Pike and Dave Cheney have discussed this in [2014](https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html) and [2016](https://dave.cheney.net/2016/11/13/do-not-fear-first-class-functions), respectively.

``` go
func SpecificThingOptionFunc func(st *SpecificThing) error

func SetHost(host string) error {
  return func(st *SpecificThing) error {
    st.host = host
    return nil
  }
}

type SpecificThing struct {
  host string
}

func NewSpecificThing(options ...SpecificThingOptionFunc) *SpecificThing {
  st := &SpecificThing{}
  for o := range options {
    o(st)
  }
  return st
}

func main() {
  st := NewSpecificThing(SetHost("example.com"))
  // ...
}
```

If a program has many _specific things_, it may have some _common things_, using [struct cmoposition](https://travix.io/type-embedding-in-go-ba40dd4264df).

``` go
type CommonThing struct {
  loggingLevel int
}

type SpecificThing struct {
  CommonThing
  host string
}
```

Continue the options func pattern, `CommonThing` can also have option funcs.

``` go
func CommonThingOptionFunc func(ct *CommonThing) error

func SetLoggingLevel(loggingLevel int) error {
  return func(ct *CommonThing) error {
    ct.loggingLevel = loggingLevel
    return nil
  }
}
```

The problem comes when the `SpecificThing` constructor wants to apply option to its `CommontThing`.  There are a couple of common patterns that can address this.

### Multiple sets of options

A constructor may accept arrays of options of different types by dropping the vardiac syntax:

``` go
func NewSpecificThing(ct CommonThing, ctOptions []CommonThingOptionFunc, stOptions []SpecificThingOptionFunc) {
  ct := CommonThing {}
  for o := range ctOptions {
    o(ct)
  }
  st := &SpecificThing {
    CommonThing: ct,
  }
  for o := range stOptions {
    o(st)
  }
  return st
}
```

This rapidly becomes inellegant and unweildy as the number of embedded structures with options grows.  For example, if `CommonThing` also embeds another struct, and that struct embeds one, etc., the argument list rapidly grows out of control.

### Dependency Injection

A constructor may require that all of its constituant parts may already exist at the time of construction:

``` go
func NewSpecificThing(ct CommonThing, options ...SpecificThingOptionFunc) {
  st := &SpecificThing {
    CommonThing: ct,
  }
  for o := range options {
    o(st)
  }
  return st
}
```

This pattern should scale more elegantly.  However, this looks a little like [dependency injection](https://blog.drewolson.org/dependency-injection-in-go), which is not appropriate for every project.

## Motivation

The goals of this project are to allow a _single_ options func to be applicable across many embedded structs.

## Usage

```
options github.com/object88/hoarding:Options,Suboptions github.com/object88/hoarding/internal:Options
```