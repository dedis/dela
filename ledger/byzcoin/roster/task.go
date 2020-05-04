package roster

import (
	"encoding/binary"
	"io"
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

const (
	// RosterValueKey is the key used to store the roster.
	RosterValueKey = "roster_value"

	// RosterChangeSetKey is the key used to store the roster change set.
	RosterChangeSetKey = "roster_changeset"

	// RosterArcKey is the ke used to store the access rights control of the
	// roster.
	RosterArcKey = "roster_arc"
)

var rosterValueKey = []byte(RosterValueKey)
var rosterChangeSetKey = []byte(RosterChangeSetKey)

// clientTask is the client task implementation to update the roster of a
// consensus using the transactions for access rights control.
//
// - implements basic.ClientTask
type clientTask struct {
	remove []uint32
}

// NewClientTask creates a new roster client task that can be used to create a
// transaction.
func NewClientTask(r []uint32) basic.ClientTask {
	return clientTask{
		remove: r,
	}
}

func (t clientTask) GetChangeSet() viewchange.ChangeSet {
	changeset := viewchange.ChangeSet{
		Remove: t.remove,
	}

	return changeset
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// client task.
func (t clientTask) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Task{
		Remove: t.remove,
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
}

// Consume implements basic.ServerTask. It executes the task and write the
// changes to the page.
func (t serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	// 1. Access rights control
	// TODO: implement

	// 2. Update the roster stored in the inventory.
	value, err := page.Read(rosterValueKey)
	if err != nil {
		return xerrors.Errorf("couldn't read roster: %v", err)
	}

	roster, err := t.rosterFactory.FromProto(value)
	if err != nil {
		return xerrors.Errorf("couldn't decode roster: %v", err)
	}

	changeset := t.GetChangeSet()
	roster = roster.Apply(changeset)

	value, err = t.encoder.Pack(roster)
	if err != nil {
		return xerrors.Errorf("couldn't encode roster: %v", err)
	}

	err = page.Write(rosterValueKey, value)
	if err != nil {
		return xerrors.Errorf("couldn't write roster: %v", err)
	}

	// 3. Store the changeset so it can be read later on.
	err = t.updateChangeSet(page)
	if err != nil {
		return xerrors.Errorf("couldn't update change set: %v", err)
	}

	return nil
}

func (t serverTask) updateChangeSet(page inventory.WritablePage) error {
	pb, err := page.Read(rosterChangeSetKey)
	if err != nil {
		return xerrors.Errorf("couldn't read from page: %v", err)
	}

	changesetpb, ok := pb.(*ChangeSet)
	if !ok || changesetpb.GetIndex() != page.GetIndex() {
		// Initialize if nil or reset if the change set comes from a previous
		// block.
		changesetpb = &ChangeSet{
			// Keep track of which index the change set is for as the inventory
			// moves values from previous pages.
			Index: page.GetIndex(),
		}
	}

	removals := append(changesetpb.GetRemove(), t.remove...)
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

	changesetpb.Remove = removals

	err = page.Write(rosterChangeSetKey, changesetpb)
	if err != nil {
		return xerrors.Errorf("couldn't write to page: %v", err)
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
}

// NewTaskManager returns a new instance of the task factory.
func NewTaskManager(f viewchange.AuthorityFactory, i inventory.Inventory) TaskManager {
	return TaskManager{
		encoder:       encoding.NewProtoEncoder(),
		inventory:     i,
		rosterFactory: f,
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

// GetChangeSet implements viewchange.Governance. It returns the change set for
// that block by reading the transactions.
func (f TaskManager) GetChangeSet(index uint64) (viewchange.ChangeSet, error) {
	cs := viewchange.ChangeSet{}

	page, err := f.inventory.GetPage(index)
	if err != nil {
		return cs, xerrors.Errorf("couldn't read page: %v", err)
	}

	pb, err := page.Read(rosterChangeSetKey)
	if err != nil {
		return cs, xerrors.Errorf("couldn't read from page: %v", err)
	}

	changesetpb, ok := pb.(*ChangeSet)
	if !ok || index != changesetpb.GetIndex() {
		// Either the change set is not defined, or it has been for a previous
		// block we return an empty change set.
		return cs, nil
	}

	cs.Remove = changesetpb.GetRemove()

	return cs, nil
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

	task := serverTask{
		clientTask: clientTask{
			remove: pb.Remove,
		},
		encoder:       f.encoder,
		rosterFactory: f.rosterFactory,
	}
	return task, nil
}
