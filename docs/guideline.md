# Programming guideline

This page covers some opiniated rules that are not (or not enought) covered by
the golang best practices.

## Files

The root implementation of a package should always be located in a `mod.go`
files.

## Comments

Any comments should be formatted at 80 chars (in VS code the *Rewrap* does the
job pretty well).

---

The implementation of an abstraction should start with `<function> implements <abstraction>. It ...`

```go
// FromText implements mino.AddressFactory. It returns an instance of an
// address from a byte slice.
func (f addressFactory) FromText(text []byte) mino.Address {
    return address{id: string(text)}
}
```

---

A struct that implements an interface should have a comment that ends with `- implements <abstraction>`:

```go
// RPC represents an RPC that has been registered by a client, which allows
// clients to call an RPC that will execute the provided handler.
//
// - implements mino.RPC
type RPC struct {
	handler mino.Handler
	srv     Server
	uri     string
}
```

## Tests

Tests should be named according to their specific scope of testing, following 
the syntax `Test<scope>(_<purpose>)_<function>`. For instance, a unit test of a function 
should follow that convention:

```go
// dummy.go

type Dummy struct{}

func (d Dummy) String() string {...}
```

```go
// dummy_test.go

func TestDummy_String(t *testing.T) {}

func TestDummy_Failures_String(t *testing.T) {}
```

In test files, all the utility stuff should be grouped at the end of the file,
preceded by

```go
// -----------------
// Utility functions

// utility stuff...
```

## Error

Don't feel guilty providing too much information in an error message. You never
really know you needed it until you experience it. The following is an example
where providing the address gives a precious information when the error occurs:

```go
neighbour, ok := srv.neighbours[addr]
if !ok {
	return nil, xerrors.Errorf("couldn't find neighbour [%s]", addr)
}
```
