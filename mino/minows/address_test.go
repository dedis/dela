package minows

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"go.dedis.ch/dela/mino"

	"github.com/stretchr/testify/require"
)

func Test_newAddress(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const pid2 = "QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"
	const addrAllInterface = "/ip4/0.0.0.0/tcp/80"
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	const addrHostname = "/dns4/example.com/tcp/80"
	type args struct {
		location string
		identity string
	}
	tests := map[string]struct {
		args args
	}{
		"all interface": {
			args: args{addrAllInterface, pid1},
		},
		"localhost": {
			args: args{addrLocalhost, pid2},
		},
		"hostname": {
			args: args{addrHostname, pid1},
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			location := mustCreateMultiaddress(t, tt.args.location)
			identity := mustCreatePeerID(t, tt.args.identity)

			_, err := newAdress(location, identity)
			require.NoError(t, err)
		})
	}
}

func Test_newAddress_Invalid(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const addrAllInterface = "/ip4/0.0.0.0/tcp/80"

	tests := map[string]struct {
		location ma.Multiaddr
		identity peer.ID
	}{
		"missing location": {
			location: nil,
			identity: mustCreatePeerID(t, pid1),
		},
		"missing identity": {
			location: mustCreateMultiaddress(t, addrAllInterface),
			identity: "",
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			_, err := newAdress(tt.location, tt.identity)
			require.Error(t, err)
		})
	}
}

func Test_address_Equal(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const pid2 = "QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	const addrHostname = "/dns4/example.com/tcp/80"
	reference := mustCreateAddress(t, addrHostname, pid1)
	tests := map[string]struct {
		self  address
		other mino.Address
		out   bool
	}{
		"self": {
			self:  reference,
			other: reference,
			out:   true,
		},
		"copy": {
			self:  reference,
			other: mustCreateAddress(t, addrHostname, pid1),
			out:   true,
		},
		"diff location": {
			self:  reference,
			other: mustCreateAddress(t, addrLocalhost, pid1),
			out:   false,
		},
		"diff identity": {
			self:  reference,
			other: mustCreateAddress(t, addrHostname, pid2),
			out:   false,
		},
		"nil": {
			self:  reference,
			other: nil,
			out:   false,
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			result := tt.self.Equal(tt.other)

			require.Equal(t, tt.out, result)
		})
	}
}

func Test_address_String(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const pid2 = "QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"
	const addrAllInterface = "/ip4/0.0.0.0/tcp/80"
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	const addrHostname = "/dns4/example.com/tcp/80"
	tests := map[string]struct {
		a    address
		want string
	}{
		"all interface": {
			a:    mustCreateAddress(t, addrAllInterface, pid1),
			want: "/ip4/0.0.0.0/tcp/80/p2p/QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU",
		},
		"localhost": {
			a:    mustCreateAddress(t, addrLocalhost, pid2),
			want: "/ip4/127.0.0.1/tcp/80/p2p/QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv",
		},
		"hostname": {
			a:    mustCreateAddress(t, addrHostname, pid1),
			want: "/dns4/example.com/tcp/80/p2p/QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU",
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			result := tt.a.String()

			require.Equal(t, tt.want, result)
		})
	}
}

func Test_address_MarshalText(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const pid2 = "QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"
	const addrAllInterface = "/ip4/0.0.0.0/tcp/80"
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	const addrHostname = "/dns4/example.com/tcp/80"
	tests := map[string]struct {
		a    address
		want []byte
	}{
		"all interface": {
			a: mustCreateAddress(t, addrAllInterface, pid1),
			want: []byte("/ip4/0.0.0.0/tcp/80" +
				":QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"),
		},
		"localhost": {
			a: mustCreateAddress(t, addrLocalhost, pid2),
			want: []byte("/ip4/127.0.0.1/tcp/80" +
				":QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"),
		},
		"hostname": {
			a: mustCreateAddress(t, addrHostname, pid1),
			want: []byte("/dns4/example.com/tcp/80" +
				":QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"),
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			result, err := tt.a.MarshalText()

			require.NoError(t, err)
			require.Equal(t, tt.want, result)
		})
	}
}

func Test_orchestratorAddr_MarshalText(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	const addrHostname = "/dns4/example.com/tcp/80"
	tests := map[string]struct {
		a    orchestratorAddr
		want []byte
	}{
		"localhost": {
			a: mustCreateOrchestratorAddr(t, addrLocalhost, pid1),
			want: []byte("/ip4/127.0.0.1/tcp/80" +
				":QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU:o"),
		},
		"hostname": {
			a: mustCreateOrchestratorAddr(t, addrHostname, pid1),
			want: []byte("/dns4/example.com/tcp/80" +
				":QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU:o"),
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			result, err := tt.a.MarshalText()

			require.NoError(t, err)
			require.Equal(t, tt.want, result)
		})
	}
}

func Test_addressFactory_FromText(t *testing.T) {
	const pid1 = "QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"
	const pid2 = "QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"
	const addrAllInterface = "/ip4/0.0.0.0/tcp/80"
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	const addrHostname = "/dns4/example.com/tcp/80"
	tests := map[string]struct {
		args []byte
		want mino.Address
	}{
		"all interface": {
			args: []byte("/ip4/0.0.0.0/tcp/80:" +
				"QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"),
			want: mustCreateAddress(t, addrAllInterface, pid1),
		},
		"localhost": {
			args: []byte("/ip4/127.0.0.1/tcp/80:" +
				"QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"),
			want: mustCreateAddress(t, addrLocalhost, pid2),
		},
		"hostname": {
			args: []byte("/dns4/example.com/tcp/80:" +
				"QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"),
			want: mustCreateAddress(t, addrHostname, pid1),
		},
		"orchestrator - localhost": {
			args: []byte("/ip4/127.0.0.1/tcp/80:" +
				"QmVt9t5Tk2uEoA4CDbKNCVxqrut8UXmWHXvFZ8wFZ3ghhv"),
			want: mustCreateAddress(t, addrLocalhost, pid2),
		},
		"orchestrator - hostname": {
			args: []byte("/dns4/example.com/tcp/80" +
				":QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU:o"),
			want: mustCreateOrchestratorAddr(t, addrHostname, pid1),
		},
	}
	factory := addressFactory{}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			result := factory.FromText(tt.args)

			require.Equal(t, tt.want, result)
		})
	}
}

func Test_addressFactory_FromText_Invalid(t *testing.T) {
	const addrLocalhost = "/ip4/127.0.0.1/tcp/80"
	tests := map[string]struct {
		args []byte
	}{
		"invalid text": {
			args: []byte("invalid"),
		},
		"missing location": {
			args: []byte(":QmaD31nEzFGwD8dK96UFWHtTYTqYJgHLMYSFz4W4Hm2WCU"),
		},
		"missing identity": {
			args: []byte(addrLocalhost),
		},
	}

	factory := addressFactory{}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			result := factory.FromText(tt.args)

			require.Nil(t, result)
		})
	}
}

func mustCreateOrchestratorAddr(t *testing.T, location, identity string) orchestratorAddr {
	addr, err := newOrchestratorAddr(mustCreateMultiaddress(t, location),
		mustCreatePeerID(t, identity))
	require.NoError(t, err)
	return addr
}

func mustCreateAddress(t *testing.T, location, identity string) address {
	addr, err := newAdress(mustCreateMultiaddress(t, location),
		mustCreatePeerID(t, identity))
	require.NoError(t, err)
	return addr
}

func mustCreateMultiaddress(t *testing.T, address string) ma.Multiaddr {
	if address == "" {
		return nil
	}
	multiaddr, err := ma.NewMultiaddr(address)
	require.NoError(t, err)
	return multiaddr
}

func mustCreatePeerID(t *testing.T, id string) peer.ID {
	pid, err := peer.Decode(id)
	require.NoError(t, err)
	return pid
}
