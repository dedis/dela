# Programming guideline

This page covers some opinionated rules that are not (or not enough) covered by
the golang best practices. We first stick to the official and common golang 
best practices, using this as a supplement.

## Files

The root implementation of a package should always be located in a `mod.go`
file.

## Comments

Any comments should be formatted at 80 chars (in VS code the *Rewrap* does the
job pretty well).

---

The implementation of an abstraction should start with the comment `<function> implements <abstraction>. It ...`

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
// -----------------------------------------------------------------------------
// Utility functions

type fakeNeededStruct{}
// ...
```

Test a function following a "bottom-top" path, ie. by first testing the entire
execution with the happy path, then by checking each possible case from the last
to the first one. Here is an example of a function and its test cases:

```go
func Dummy() error {
	if x {
		return error.New("error 1")
	}
	if y {
		return error.New("error 2")
	}
	
	return nil
}

func TestDummy(t *testing.T) {
	err := dummy()
	require.NoError(t, err)
	
	err = dummy()
	require.EqualError(t, err, "error 2")
	
	err = dummy()
	require.EqualError(t, err, "error 1")
}
```

## Error

Don't feel guilty providing too much information in an error message. You never
really know you needed it until you experience it.

```go
// DON'T
neighbour, ok := srv.neighbours[addr]
if !ok {
	return nil, xerrors.Errorf("finding neighbour")
}
// This could result in a BAD error message like:
// > Failed to start server: finding neighbour

// DO
neighbour, ok := srv.neighbours[addr]
if !ok {
	return nil, xerrors.Errorf("couldn't find neighbour [%s]", addr)
}
// This could result in a GOOD error message like:
// > Failed to start server: couldn't find neighbour [127.0.0.1]
```

## Misc

A "constructor" function should be named `New<struct name>` and placed right after the struct's declaration:

```go
type Dummy struct{
	// ...
}

func NewDummy(...) Dummy {
	// ...
}
```

Try to use blank lines between groups of instructions. Don't spare lines:

```go
// DON'T
func (g *simpleGatherer) Wait(ctx context.Context, cfg Config) []txn.Transaction {
	ch := make(chan []txn.Transaction, 1)
	g.Lock()
	g.queue = append(g.queue, item{cfg: cfg, ch: ch})
	g.Unlock()
	if cfg.Callback != nil {
		cfg.Callback()
	}
	select {
	case txs := <-ch:
		return txs
	case <-ctx.Done():
		return nil
	}
}

// DO
func (g *simpleGatherer) Wait(ctx context.Context, cfg Config) []txn.Transaction {
	ch := make(chan []txn.Transaction, 1)

	g.Lock()
	g.queue = append(g.queue, item{cfg: cfg, ch: ch})
	g.Unlock()

	if cfg.Callback != nil {
		cfg.Callback()
	}

	select {
	case txs := <-ch:
		return txs
	case <-ctx.Done():
		return nil
	}
}
```

Don't use `if` with initialization statements. It makes the code harder to read.
Seriously, don't spare the extra line.

```go
// DON'T
if err := dummy(); err != nil {
	// ...
}

// DO
err := dummy()
if err != nil {
	// ...
}
```

Try at all costs not to use initialized return variables. It makes the code harder to read.

```go
// DON'T
func dummy() (els []int, err error) {
	// ...
	return // <- what is returned exactly?
}

// DO
func dummy() ([]int, error) {
	// ...
	return result, nil
}
```

If you initialize a struct, try to first gather your needed elements and then set it up once:

```go
// DON'T
func NewPedersen(m mino.Mino) (*Pedersen) {
	p := new(Pedersen)
	p.factory = types.NewMessageFactory(m.GetAddressFactory())
	p.privkey = suite.Scalar().Pick(suite.RandomStream())
	p.mino = m

	return p
}

// DO
func NewPedersen(m mino.Mino) (*Pedersen) {
	factory := types.NewMessageFactory(m.GetAddressFactory())
	privkey := suite.Scalar().Pick(suite.RandomStream())

	return &Pedersen{
		privKey: privkey,
		mino:    m,
		factory: factory,
	}
}
```
