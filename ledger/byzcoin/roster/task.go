package roster

import (
	"encoding/binary"
	"io"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/consensus/viewchange"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/inventory"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"golang.org/x/xerrors"
)

const (
	// AuthorityKey is the key used to store the roster.
	AuthorityKey = "authority:value"

	// ArcKey is the ke used to store the access rights control of the roster.
	ArcKey = "authority:arc"
)

var authorityKey = []byte(AuthorityKey)
var changesetKey = []byte("authority:changeset")

// clientTask is the client task implementation to update the roster of a
// consensus using the transactions for access rights control.
type clientTask struct {
	remove []uint32
}

// NewClientTask creates a new roster client task that can be used to create a
// transaction.
func NewClientTask(r []uint32) basic.ClientAction {
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
	pb := &ActionProto{
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
type serverTask struct {
	clientTask
	encoder       encoding.ProtoMarshaler
	rosterFactory viewchange.AuthorityFactory
}

// Consume implements basic.ServerAction. It executes the task and write the
// changes to the page.
func (t serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	// 1. Access rights control
	// TODO: implement

	// 2. Update the roster stored in the inventory.
	value, err := page.Read(authorityKey)
	if err != nil {
		return err
	}

	roster, err := t.rosterFactory.FromProto(value)
	if err != nil {
		return err
	}

	changeset := t.GetChangeSet()
	roster = roster.Apply(changeset)

	value, err = t.encoder.Pack(roster)
	if err != nil {
		return err
	}

	err = page.Write(authorityKey, value)
	if err != nil {
		return err
	}

	// 3. Store the changeset so it can be read later on.
	changesetpb := &ChangeSet{
		Remove: changeset.Remove,
	}

	err = page.Write(changesetKey, changesetpb)
	if err != nil {
		return err
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

// NewTaskManager returns a new instance of the action factory.
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

	rosterpb, err := page.Read(authorityKey)
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

	pb, err := page.Read(changesetKey)
	if err != nil {
		return cs, nil
	}

	changesetpb, ok := pb.(*ChangeSet)
	if !ok {
		return cs, nil
	}

	cs.Remove = changesetpb.GetRemove()

	return cs, nil
}

// FromProto implements basic.ActionFactory.
func (f TaskManager) FromProto(in proto.Message) (basic.ServerAction, error) {
	var pb *ActionProto
	switch msg := in.(type) {
	case *ActionProto:
		pb = msg
	}

	action := serverTask{
		clientTask: clientTask{
			remove: pb.Remove,
		},
		encoder:       f.encoder,
		rosterFactory: f.rosterFactory,
	}
	return action, nil
}
