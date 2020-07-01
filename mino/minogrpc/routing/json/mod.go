package json

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	routing.Register(serde.CodecJSON, format{})
}

// Address is the JSON format of an address.
type Address []byte

// TreeRouting is the JSON message for a tree routing.
type TreeRouting struct {
	Root      int
	Addresses []Address
}

// Format is the implementation of the JSON format for a tree routing.
//
// - implements serde.Format
type format struct{}

// Encode implements serde.Format. It serializes the given routing in JSON.
func (f format) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	rting, ok := msg.(routing.TreeRouting)
	if !ok {
		return nil, xerrors.New("invalid routing")
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
		return nil, err
	}

	return data, nil
}

func (f format) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := TreeRouting{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	factory := ctx.GetFactory(routing.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid address factory of type '%T'", factory)
	}

	addrs := make([]mino.Address, len(m.Addresses))
	for i, addr := range m.Addresses {
		addrs[i] = fac.FromText(addr)
	}

	rting, err := routing.NewTreeRouting(mino.NewAddresses(addrs...), routing.WithRootAt(m.Root))
	if err != nil {
		return nil, err
	}

	return rting, nil
}
