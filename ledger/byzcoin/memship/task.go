package memship

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/wrappers"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	// RosterValueKey is the key used to store the roster.
	RosterValueKey = "roster_value"

	// RosterArcKey is the ke used to store the access rights control of the
	// roster.
	RosterArcKey = "roster_arc"

	// RosterLeaderKey is the key where the leader is stored.
	RosterLeaderKey = "roster_leader"
)

var (
	rosterValueKey = []byte(RosterValueKey)

	rosterLeaderKey = []byte(RosterLeaderKey)
)

// clientTask is the client task implementation to update the roster of a
// consensus using the transactions for access rights control.
//
// - implements basic.ClientTask
type clientTask struct {
	remove []uint32
	player *viewchange.Player
}

// NewRemove creates a new roster client task that will remove a player from the
// roster.
func NewRemove(r []uint32) basic.ClientTask {
	return clientTask{
		remove: r,
	}
}

// NewAdd creates a new roster client task that will add a player to the roster.
func NewAdd(addr mino.Address, pubkey crypto.PublicKey) basic.ClientTask {
	player := viewchange.Player{
		Address:   addr,
		PublicKey: pubkey,
	}

	return clientTask{player: &player}
}

func (t clientTask) GetChangeSet() viewchange.ChangeSet {
	removals := append([]uint32{}, t.remove...)
	// Sort by ascending order in O(n*log(n)).
	sort.Slice(removals, func(i, j int) bool { return removals[i] > removals[j] })
	// Remove duplicates in O(n).
	for i := 0; i < len(removals)-1; {
		if removals[i] == removals[i+1] {
			removals = append(removals[:i], removals[i+1:]...)
		} else {
			// Only moves to the next when all occurences of the same index are
			// removed.
			i++
		}
	}

	changeset := viewchange.ChangeSet{
		Remove: removals,
	}

	if t.player != nil {
		changeset.Add = []viewchange.Player{*t.player}
	}

	return changeset
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// client task.
func (t clientTask) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Task{
		Remove: t.remove,
	}

	if t.player != nil {
		addr, err := t.player.Address.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal address: %v", err)
		}

		pubkey, err := enc.PackAny(t.player.PublicKey)
		if err != nil {
			return nil, xerrors.Errorf("couldn't pack public key: %v", err)
		}

		pb.Addr = addr
		pb.PublicKey = pubkey
	}

	return pb, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// to the writer in a deterministic way.
func (t clientTask) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
	buffer := make([]byte, 4*len(t.remove))
	for i, index := range t.remove {
		binary.LittleEndian.PutUint32(buffer[i*4:], index)
	}

	_, err := w.Write(buffer)
	if err != nil {
		return xerrors.Errorf("couldn't write remove indices: %v", err)
	}

	if t.player != nil {
		buffer, err = t.player.Address.MarshalText()
		if err != nil {
			return xerrors.Errorf("couldn't marshal address: %v", err)
		}

		_, err = w.Write(buffer)
		if err != nil {
			return xerrors.Errorf("couldn't write address: %v", err)
		}

		buffer, err = t.player.PublicKey.MarshalBinary()
		if err != nil {
			return xerrors.Errorf("couldn't marshal public key: %v", err)
		}

		_, err = w.Write(buffer)
		if err != nil {
			return xerrors.Errorf("couldn't write public key: %v", err)
		}
	}

	return nil
}

// serverTask is the extension of the client task to consume the task and update
// the inventory page accordingly.
//
// - implements basic.ServerTask
type serverTask struct {
	clientTask
	encoder       encoding.ProtoMarshaler
	rosterFactory roster.Factory
	inventory     inventory.Inventory
}

// Consume implements basic.ServerTask. It executes the task and write the
// changes to the page. If multiple roster transactions are executed for the
// same page, only the latest will be taken in account.
func (t serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	// 1. Access rights control
	// TODO: implement

	// 2. Update the roster stored in the inventory.
	prev, err := t.inventory.GetPage(page.GetIndex() - 1)
	if err != nil {
		return xerrors.Errorf("couldn't get previous page: %v", err)
	}

	value, err := prev.Read(rosterValueKey)
	if err != nil {
		return xerrors.Errorf("couldn't read roster: %v", err)
	}

	roster, err := t.rosterFactory.FromProto(value)
	if err != nil {
		return xerrors.Errorf("couldn't decode roster: %v", err)
	}

	roster = roster.Apply(t.GetChangeSet())

	value, err = t.encoder.Pack(roster)
	if err != nil {
		return xerrors.Errorf("couldn't encode roster: %v", err)
	}

	err = page.Write(rosterValueKey, value)
	if err != nil {
		return xerrors.Errorf("couldn't write roster: %v", err)
	}

	return nil
}

// TaskManager manages the roster tasks by providing a factory and a governance
// implementation.
//
// - implements basic.TaskManager
// - implements viewchange.Governance
type TaskManager struct {
	me            mino.Address
	encoder       encoding.ProtoMarshaler
	inventory     inventory.Inventory
	rosterFactory roster.Factory
}

// NewTaskManager returns a new instance of the task factory.
func NewTaskManager(i inventory.Inventory, m mino.Mino, s crypto.Signer) TaskManager {
	return TaskManager{
		me:            m.GetAddress(),
		encoder:       encoding.NewProtoEncoder(),
		inventory:     i,
		rosterFactory: roster.NewRosterFactory(m.GetAddressFactory(), s.GetPublicKeyFactory()),
	}
}

// GetGenesis implements viewchange.ViewChange.
func (f TaskManager) GetGenesis() (viewchange.Authority, error) {
	page, err := f.inventory.GetPage(0)
	if err != nil {
		return nil, err
	}

	roster, err := f.readAuthority(page)
	if err != nil {
		return nil, err
	}

	return roster, nil
}

// GetAuthority implements viewchange.ViewChange. It returns the current
// authority based of the last page of the inventory.
func (f TaskManager) GetAuthority() (viewchange.Authority, error) {
	page, err := f.inventory.GetPage(f.inventory.Len() - 1)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read page: %v", err)
	}

	roster, err := f.readAuthority(page)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read roster: %v", err)
	}

	return roster, nil
}

func (f TaskManager) readAuthority(page inventory.Page) (viewchange.Authority, error) {
	rosterpb, err := page.Read(rosterValueKey)
	if err != nil {
		return nil, xerrors.Errorf("reading entry: %v", err)
	}

	roster, err := f.rosterFactory.FromProto(rosterpb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode roster: %v", err)
	}

	return roster, nil
}

// Wait implements viewchange.ViewChange. It returns true if the node is the
// leader for the current authority.
func (f TaskManager) Wait() bool {
	curr, err := f.GetAuthority()
	if err != nil {
		return false
	}

	iter := curr.AddressIterator()
	return iter.HasNext() && iter.GetNext().Equal(f.me)
}

// Verify implements viewchange.ViewChange. It returns the previous authority
// for the latest page and the current proposed authority. If the given address
// is not the leader, it will return an error.
func (f TaskManager) Verify(from mino.Address) (viewchange.Authority, viewchange.Authority, error) {
	curr, err := f.GetAuthority()
	if err != nil {
		return nil, nil, err
	}

	iter := curr.AddressIterator()
	if !iter.HasNext() || !iter.GetNext().Equal(from) {
		return nil, nil, xerrors.New("mismatch leader")
	}

	leader, err := from.MarshalText()
	if err != nil {
		return nil, nil, err
	}

	var staged inventory.Page

	f.inventory.Range(func(p inventory.Page) bool {
		value, err := p.Read(rosterLeaderKey)
		if err != nil {
			dela.Logger.Warn().Err(err).Msg("roster leader not in page")
			return true
		}

		if bytes.Equal(value.(*wrappers.BytesValue).Value, leader) {
			staged = p
			return false
		}

		return true
	})

	if staged != nil {
		next, err := f.readAuthority(staged)
		if err != nil {
			return nil, nil, xerrors.Errorf("couldn't read next authority: %v", err)
		}

		return curr, next, nil
	}

	return curr, curr, nil
}

// FromProto implements basic.TaskFactory. It returns the server task associated
// with the server task if appropriate, otherwise an error.
func (f TaskManager) FromProto(in proto.Message) (basic.ServerTask, error) {
	var pb *Task
	switch msg := in.(type) {
	case *Task:
		pb = msg
	case *any.Any:
		pb = &Task{}
		err := f.encoder.UnmarshalAny(msg, pb)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
		}
	default:
		return nil, xerrors.Errorf("invalid message type '%T'", in)
	}

	player, err := f.unpackPlayer(pb.GetAddr(), pb.GetPublicKey())
	if err != nil {
		return nil, xerrors.Errorf("couldn't unpack player: %v", err)
	}

	task := serverTask{
		clientTask: clientTask{
			remove: pb.Remove,
			player: player,
		},
		encoder:       f.encoder,
		rosterFactory: f.rosterFactory,
		inventory:     f.inventory,
	}
	return task, nil
}

func (f TaskManager) unpackPlayer(addrpb []byte,
	pubkeypb proto.Message) (*viewchange.Player, error) {

	if addrpb == nil || pubkeypb == nil {
		return nil, nil
	}

	addr := f.rosterFactory.GetAddressFactory().FromText(addrpb)

	pubkey, err := f.rosterFactory.GetPublicKeyFactory().FromProto(pubkeypb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode public key: %v", err)
	}

	player := viewchange.Player{
		Address:   addr,
		PublicKey: pubkey,
	}

	return &player, nil
}
