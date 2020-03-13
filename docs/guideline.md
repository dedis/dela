# Programming guideline

This page covers some opiniated rules that are not (or not enought) covered by
the golang best practices.

## Files

The root implementation of a package should always be located in a `go.mod`
files.

## Comments

Any comments should be formatted at 80 chars (in VS code the *Rewrap* does the
job pretty well).

---

The implementation of an abstraction should start with `<function> implements <abstraction>. It ...`

```go
// FromText implements AddressFactory.FromText(). It returns an instance of an
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