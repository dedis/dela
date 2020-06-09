package darc

import (
	"bytes"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Expression{},
		&AccessProto{},
		&Task{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestAccess_Evolve(t *testing.T) {
	access := NewAccess()

	idents := []arc.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	access, err := access.Evolve("fake", idents...)
	require.NoError(t, err)
	require.Len(t, access.rules, 1)

	access, err = access.Evolve("another", idents...)
	require.NoError(t, err)
	require.Len(t, access.rules, 2)

	access, err = access.Evolve("fake")
	require.NoError(t, err)
	require.Len(t, access.rules, 2)

	_, err = access.Evolve("fake", fakeIdentity{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't evolve rule: couldn't marshal identity: oops")
}

func TestAccess_Match(t *testing.T) {
	idents := []arc.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	access, err := NewAccess().Evolve("fake", idents...)
	require.NoError(t, err)

	err = access.Match("fake", idents...)
	require.NoError(t, err)

	err = access.Match("fake")
	require.EqualError(t, err, "expect at least one identity")

	err = access.Match("unknown", idents...)
	require.EqualError(t, err, "rule 'unknown' not found")

	err = access.Match("fake", fakeIdentity{buffer: []byte{0xcc}})
	require.EqualError(t, err,
		"couldn't match 'fake': couldn't match identity '\xcc'")
}

func TestAccess_Fingerprint(t *testing.T) {
	access := Access{
		rules: map[string]expression{
			"\x02": {matches: map[string]struct{}{"\x04": {}}},
		},
	}

	buffer := new(bytes.Buffer)

	err := access.Fingerprint(buffer, nil)
	require.NoError(t, err)
	require.Equal(t, "\x02\x04", buffer.String())

	err = access.Fingerprint(fake.NewBadHash(), nil)
	require.EqualError(t, err, "couldn't write key: fake error")

	err = access.Fingerprint(fake.NewBadHashWithDelay(1), nil)
	require.EqualError(t, err,
		"couldn't fingerprint rule '\x02': couldn't write match: fake error")
}

func TestAccess_Pack(t *testing.T) {
	idents := []arc.Identity{
		fakeIdentity{buffer: []byte{0xaa}},
		fakeIdentity{buffer: []byte{0xbb}},
	}

	access, err := NewAccess().Evolve("fake", idents...)
	require.NoError(t, err)

	encoder := encoding.NewProtoEncoder()

	pb, err := access.Pack(encoder)
	require.NoError(t, err)
	require.Len(t, pb.(*AccessProto).GetRules(), 1)

	_, err = access.Pack(fake.BadPackEncoder{})
	require.EqualError(t, err, "couldn't pack expression: fake error")
}

func TestAccess_VisitJSON(t *testing.T) {
	access := Access{rules: map[string]expression{
		"A": {matches: map[string]struct{}{"C": {}}},
		"B": {},
	}}

	ser := json.NewSerializer()
	data, err := ser.Serialize(access)
	require.NoError(t, err)
	require.Equal(t, `{"Rules":{"A":["C"],"B":[]}}`, string(data))
}

func TestFactory_FromProto(t *testing.T) {
	factory := NewFactory()

	pb := &AccessProto{
		Rules: map[string]*Expression{
			"fake": {
				Matches: []string{"aa", "bb"},
			},
		},
	}

	access, err := factory.FromProto(pb)
	require.NoError(t, err)
	require.NotNil(t, access)

	pbAny, err := ptypes.MarshalAny(pb)
	require.NoError(t, err)
	access, err = factory.FromProto(pbAny)
	require.NoError(t, err)
	require.NotNil(t, access)

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "invalid message type '*empty.Empty'")

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(pbAny)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")
}

func TestFactory_VisitJSON(t *testing.T) {
	factory := NewFactory()

	ser := json.NewSerializer()

	var access Access
	err := ser.Deserialize([]byte(`{"Rules":{"A":["B"],"C":[]}}`), factory, &access)
	require.NoError(t, err)
	expected := Access{rules: map[string]expression{
		"A": {matches: map[string]struct{}{"B": {}}},
		"C": {matches: map[string]struct{}{}},
	}}
	require.Equal(t, expected, access)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize access: fake error")
}
