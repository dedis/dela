package memship

import (
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/ledger/byzcoin/memship/json"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	// RosterValueKey is the key used to store the roster.
	RosterValueKey = "roster_value"

	// RosterArcKey is the ke used to store the access rights control of the
	// roster.
	RosterArcKey = "roster_arc"
)

var (
	rosterValueKey = []byte(RosterValueKey)
)

// clientTask is the client task implementation to update the roster of a
// consensus using the transactions for access rights control.
//
// - implements basic.clientTask
type clientTask struct {
	serde.UnimplementedMessage

	authority viewchange.Authority
}

// NewTask returns a new client task to update the authority.
func NewTask(authority crypto.CollectiveAuthority) basic.ClientTask {
	return clientTask{authority: roster.New(authority)}
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// client task.
func (t clientTask) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	authority, err := enc.PackAny(t.authority)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack authority: %v", err)
	}

	pb := &Task{Authority: authority}

	return pb, nil
}

// VisitJSON implements serde.Message. It serializes the client task in JSON
// format.
func (t clientTask) VisitJSON(ser serde.Serializer) (interface{}, error) {
	authority, err := ser.Serialize(t.authority)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize authority: %v", err)
	}

	m := json.Task{
		Authority: authority,
	}

	return m, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// to the writer in a deterministic way.
func (t clientTask) Fingerprint(w io.Writer, e encoding.ProtoMarshaler) error {
	err := t.authority.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("couldn't fingerprint authority: %v", err)
	}

	return nil
}

// serverTask is the extension of the client task to consume the task and update
// the inventory page accordingly.
//
// - implements basic.ServerTask
type serverTask struct {
	clientTask
	encoder encoding.ProtoMarshaler
}

// Consume implements basic.ServerTask. It executes the task and write the
// changes to the page. If multiple roster transactions are executed for the
// same page, only the latest will be taken in account.
func (t serverTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	// 1. Access rights control
	// TODO: implement

	// 2. Update the roster stored in the inventory.
	value, err := t.encoder.Pack(roster.New(t.authority))
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
	serde.UnimplementedFactory

	me            mino.Address
	encoder       encoding.ProtoMarshaler
	inventory     inventory.Inventory
	rosterFactory roster.Factory
	csFactory     serde.Factory
}

// NewTaskManager returns a new instance of the task factory.
func NewTaskManager(i inventory.Inventory, m mino.Mino, s crypto.Signer) TaskManager {
	return TaskManager{
		me:            m.GetAddress(),
		encoder:       encoding.NewProtoEncoder(),
		inventory:     i,
		rosterFactory: roster.NewRosterFactory(m.GetAddressFactory(), s.GetPublicKeyFactory()),
		csFactory:     roster.NewChangeSetFactory(m.GetAddressFactory(), s.GetPublicKeyFactory()),
	}
}

// GetChangeSetFactory implements viewchange.ViewChange.
func (f TaskManager) GetChangeSetFactory() serde.Factory {
	return f.csFactory
}

// GetAuthority implements viewchange.ViewChange. It returns the current
// authority based of the last page of the inventory.
func (f TaskManager) GetAuthority(index uint64) (viewchange.Authority, error) {
	page, err := f.inventory.GetPage(index)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read page: %v", err)
	}

	rosterpb, err := page.Read(rosterValueKey)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read entry: %v", err)
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
	curr, err := f.GetAuthority(f.inventory.Len() - 1)
	if err != nil {
		return false
	}

	iter := curr.AddressIterator()
	return iter.HasNext() && iter.GetNext().Equal(f.me)
}

// Verify implements viewchange.ViewChange. It returns the previous authority
// for the latest page and the current proposed authority. If the given address
// is not the leader, it will return an error.
func (f TaskManager) Verify(from mino.Address, index uint64) (viewchange.Authority, error) {
	curr, err := f.GetAuthority(index)
	if err != nil {
		return nil, xerrors.Errorf("couldn't get authority: %v", err)
	}

	iter := curr.AddressIterator()
	if !iter.HasNext() || !iter.GetNext().Equal(from) {
		return nil, xerrors.Errorf("<%v> is not the leader", from)
	}

	return curr, nil
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

	roster, err := f.rosterFactory.FromProto(pb.GetAuthority())
	if err != nil {
		return nil, xerrors.Errorf("couldn't unpack player: %v", err)
	}

	task := serverTask{
		clientTask: clientTask{
			authority: roster,
		},
		encoder: f.encoder,
	}

	return task, nil
}

// VisitJSON implements serde.Factory. It deserializes the client task in JSON
// format into a server task.
func (f TaskManager) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Task{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize task: %v", err)
	}

	var roster viewchange.Authority
	err = in.GetSerializer().Deserialize(m.Authority, f.rosterFactory, &roster)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize roster: %v", err)
	}

	task := serverTask{
		clientTask: clientTask{
			authority: roster,
		},
		encoder: f.encoder,
	}

	return task, nil
}

// Register registers the task messages.
func Register(r basic.TransactionFactory, f basic.TaskFactory) {
	r.Register(clientTask{}, f)
	r.Register(serverTask{}, f)
}
