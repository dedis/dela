package json

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	routing.Register(serde.FormatJSON, newRtingFormat())
}

// Address is the JSON format of an address.
type Address []byte

// TreeRouting is the JSON message for a tree routing.
type TreeRouting struct {
	Root      int
	Addresses []Address
}

// RtingFormat is the implementation of the JSON rtingFormat for a tree routing.
//
// - implements serde.FormatEngine
type rtingFormat struct {
	height int
}

func newRtingFormat() rtingFormat {
	return rtingFormat{
		height: 3,
	}
}

// Encode implements serde.FormatEngine. It serializes the given routing in
// JSON.
func (f rtingFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	rting, ok := msg.(routing.TreeRouting)
	if !ok {
		return nil, xerrors.Errorf("found '%T' but expected '%T'", msg, rting)
	}

	addrs := rting.GetAddresses()

	buffers := make([]Address, len(addrs))
	root := 0

	for i, addr := range addrs {
		addrBuf, err := addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal address: %v", err)
		}

		buffers[i] = addrBuf

		if addr.Equal(rting.GetRoot()) {
			root = i
		}
	}

	m := TreeRouting{
		Root:      root,
		Addresses: buffers,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal to JSON: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the routing associated
// with the data if appropriate, otherwise it returns an error.
func (f rtingFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := TreeRouting{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	factory := ctx.GetFactory(routing.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("found factory '%T'", factory)
	}

	addrs := make([]mino.Address, len(m.Addresses))
	for i, addr := range m.Addresses {
		addrs[i] = fac.FromText(addr)
	}

	opts := []routing.TreeRoutingOption{
		routing.WithRootAt(m.Root),
		routing.WithHeight(f.height),
	}

	rting, err := routing.NewTreeRouting(mino.NewAddresses(addrs...), opts...)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create tree routing: %v", err)
	}

	return rting, nil
}
