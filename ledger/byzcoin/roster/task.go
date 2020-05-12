package roster

import (
	"encoding/binary"
	"io"
	"sort"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

const (
	// RosterValueKey is the key used to store the roster.
	RosterValueKey = "roster_value"

	// RosterArcKey is the ke used to store the access rights control of the
	// roster.
	RosterArcKey = "roster_arc"
)

var rosterValueKey = []byte(RosterValueKey)

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
			// Only moves to the next when all occurances of the same index are
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
	rosterFactory viewchange.AuthorityFactory
	bag           *changeSetBag
	inventory     inventory.Inventory
}

// Consume implements basic.ServerTask. It executes the task and write the
// changes to the page. If multiple roster transactions are executed for the
// same page, only the latest will be taken in account.
func (t serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	// 1. Access rights control
	// TODO: implement

	// 2. Store the changeset so it can be read later on.
	changeset := t.GetChangeSet()

	page.Defer(func(fingerprint []byte) {
		t.bag.Stage(page.GetIndex(), fingerprint, changeset)
	})

	// 3. Update the roster stored in the inventory.
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

	roster = roster.Apply(changeset)

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
	encoder       encoding.ProtoMarshaler
	inventory     inventory.Inventory
	rosterFactory viewchange.AuthorityFactory
	bag           *changeSetBag
}

// NewTaskManager returns a new instance of the task factory.
func NewTaskManager(f viewchange.AuthorityFactory, i inventory.Inventory) TaskManager {
	return TaskManager{
		encoder:       encoding.NewProtoEncoder(),
		inventory:     i,
		rosterFactory: f,
		bag:           &changeSetBag{},
	}
}

// GetAuthorityFactory implements viewchange.AuthorityFactory. It returns the
// authority factory.
func (f TaskManager) GetAuthorityFactory() viewchange.AuthorityFactory {
	return f.rosterFactory
}

// GetAuthority implements viewchange.Governance. It returns the authority for
// the given block index by reading the inventory page associated.
func (f TaskManager) GetAuthority(index uint64) (viewchange.EvolvableAuthority, error) {
	page, err := f.inventory.GetPage(index)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read page: %v", err)
	}

	rosterpb, err := page.Read(rosterValueKey)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read roster: %v", err)
	}

	roster, err := f.rosterFactory.FromProto(rosterpb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode roster: %v", err)
	}

	return roster, nil
}

// Payload is a specific payload interface to differentiate the change sets.
type Payload interface {
	GetFingerprint() []byte
}

// GetChangeSet implements viewchange.Governance. It returns the change set for
// that block by reading the transactions.
func (f TaskManager) GetChangeSet(prop consensus.Proposal) (viewchange.ChangeSet, error) {
	var changeset viewchange.ChangeSet

	block, ok := prop.(blockchain.Block)
	if !ok {
		return changeset, xerrors.Errorf("proposal must implement blockchain.Block")
	}

	payload, ok := block.GetPayload().(Payload)
	if !ok {
		return changeset, xerrors.New("payload must implement roster.Payload")
	}

	changeset = f.bag.Get(prop.GetIndex(), payload.GetFingerprint())

	return changeset, nil
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
		// The error is not wrap to avoid redundancy with the private function.
		return nil, err
	}

	task := serverTask{
		clientTask: clientTask{
			remove: pb.Remove,
			player: player,
		},
		encoder:       f.encoder,
		rosterFactory: f.rosterFactory,
		bag:           f.bag,
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

type changeSetBag struct {
	sync.Mutex
	index uint64
	store map[[32]byte]viewchange.ChangeSet
}

func (bag *changeSetBag) Stage(index uint64, id []byte, cs viewchange.ChangeSet) {
	bag.Lock()
	defer bag.Unlock()

	if bag.index != index {
		// Reset previous staged change sets from the previous index.
		bag.store = make(map[[32]byte]viewchange.ChangeSet)
		bag.index = index
	}

	key := [32]byte{}
	copy(key[:], id)

	bag.store[key] = cs
}

func (bag *changeSetBag) Get(index uint64, fingerprint []byte) viewchange.ChangeSet {
	bag.Lock()
	defer bag.Unlock()

	if bag.index != index {
		return viewchange.ChangeSet{}
	}

	key := [32]byte{}
	copy(key[:], fingerprint)

	return bag.store[key]
}
