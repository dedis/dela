package skipchain

import (
	fmt "fmt"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"golang.org/x/xerrors"
)

func TestBlockValidator_Validate(t *testing.T) {
	f := func(block SkipBlock) bool {
		packed, err := block.Pack()
		require.NoError(t, err)

		v := &blockValidator{
			validator: fakeValidator{},
			Skipchain: &Skipchain{
				db:        fakeDatabase{genesisID: block.GenesisID},
				cosi:      fakeCosi{},
				mino:      fakeMino{},
				consensus: fakeConsensus{},
			},
		}
		prop, err := v.Validate(packed)
		require.NoError(t, err)
		require.NotNil(t, prop)
		require.Equal(t, block.GetHash(), prop.GetHash())
		require.Equal(t, block.BackLink.Bytes(), prop.GetPreviousHash())

		_, err = v.Validate(nil)
		require.EqualError(t, err, "couldn't decode block: invalid message type '<nil>'")

		v.Skipchain.db = fakeDatabase{err: xerrors.New("oops")}
		_, err = v.Validate(packed)
		require.EqualError(t, err, "couldn't read genesis block: oops")

		v.Skipchain.db = fakeDatabase{genesisID: Digest{}}
		_, err = v.Validate(packed)
		require.EqualError(t, err,
			fmt.Sprintf("mismatch genesis hash '%v' != '%v'", Digest{}, block.GenesisID))

		v.Skipchain.db = fakeDatabase{genesisID: block.GenesisID}
		v.validator = fakeValidator{err: xerrors.New("oops")}
		_, err = v.Validate(packed)
		require.EqualError(t, err, "couldn't validate the payload: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockValidator_Commit(t *testing.T) {
	v := &blockValidator{
		validator: fakeValidator{},
		Skipchain: &Skipchain{db: fakeDatabase{}},
	}

	v.buffer = SkipBlock{hash: Digest{1, 2, 3}}
	err := v.Commit(Digest{1, 2, 3}.Bytes())
	require.NoError(t, err)

	err = v.Commit([]byte{0xaa})
	require.EqualError(t, err, "unknown block aa")

	v.Skipchain.db = fakeDatabase{err: xerrors.New("oops")}
	err = v.Commit(Digest{1, 2, 3}.Bytes())
	require.EqualError(t, err, "couldn't persist the block: oops")

	v.Skipchain.db = fakeDatabase{}
	v.validator = fakeValidator{err: xerrors.New("oops")}
	err = v.Commit(Digest{1, 2, 3}.Bytes())
	require.EqualError(t, err, "couldn't commit the payload: oops")
}

type fakeValidator struct {
	blockchain.Validator
	err error
}

func (v fakeValidator) Validate(proto.Message) error {
	return v.err
}

func (v fakeValidator) Commit(proto.Message) error {
	return v.err
}

type fakeDatabase struct {
	Database
	genesisID Digest
	err       error
}

func (db fakeDatabase) Read(index int64) (SkipBlock, error) {
	return SkipBlock{hash: db.genesisID}, db.err
}

func (db fakeDatabase) Write(SkipBlock) error {
	return db.err
}

func (db fakeDatabase) ReadLast() (SkipBlock, error) {
	return SkipBlock{hash: db.genesisID}, db.err
}
