package memship

import (
	"io"

	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/ledger/inventory"
	"go.dedis.ch/dela/ledger/transactions/basic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
	"golang.org/x/xerrors"
)

const (
	// RosterValueKey is the key used to store the roster.
	RosterValueKey = "roster_value"

	// RosterArcKey is the ke used to store the access rights control of the
	// roster.
	RosterArcKey = "roster_arc"
)

var (
	rosterValueKey = []byte(RosterValueKey)

	taskFormats = registry.NewSimpleRegistry()
)

func RegisterTaskFormat(c serdeng.Codec, f serdeng.Format) {
	taskFormats.Register(c, f)
}

// ClientTask is the client task implementation to update the roster of a
// consensus using the transactions for access rights control.
//
// - implements basic.ClientTask
type ClientTask struct {
	authority viewchange.Authority
}

// NewTask returns a new client task to update the authority.
func NewTask(authority crypto.CollectiveAuthority) basic.ClientTask {
	return ClientTask{authority: roster.FromAuthority(authority)}
}

func (t ClientTask) GetAuthority() viewchange.Authority {
	return t.authority
}

// Serialize implements serde.Message.
func (t ClientTask) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := taskFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, t)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Fingerprint implements encoding.Fingerprinter. It serializes the client task
// to the writer in a deterministic way.
func (t ClientTask) Fingerprint(w io.Writer) error {
	err := t.authority.Fingerprint(w)
	if err != nil {
		return xerrors.Errorf("couldn't fingerprint authority: %v", err)
	}

	return nil
}

// ServerTask is the extension of the client task to consume the task and update
// the inventory page accordingly.
//
// - implements basic.ServerTask
type ServerTask struct {
	ClientTask
}

func NewServerTask(authority viewchange.Authority) ServerTask {
	return ServerTask{
		ClientTask: ClientTask{
			authority: authority,
		},
	}
}

// Consume implements basic.ServerTask. It executes the task and write the
// changes to the page. If multiple roster transactions are executed for the
// same page, only the latest will be taken in account.
func (t ServerTask) Consume(ctx basic.Context, page inventory.WritablePage) error {
	// 1. Access rights control
	// TODO: implement

	// 2. Update the roster stored in the inventory.
	err := page.Write(rosterValueKey, t.authority)
	if err != nil {
		return xerrors.Errorf("couldn't write roster: %v", err)
	}

	return nil
}

type RosterKey struct{}

// TaskManager manages the roster tasks by providing a factory and a governance
// implementation.
//
// - implements basic.TaskManager
// - implements viewchange.Governance
type TaskManager struct {
	serde.UnimplementedFactory

	me            mino.Address
	inventory     inventory.Inventory
	rosterFactory viewchange.AuthorityFactory
	csFactory     viewchange.ChangeSetFactory
}

// NewTaskManager returns a new instance of the task factory.
func NewTaskManager(i inventory.Inventory, m mino.Mino, s crypto.Signer) TaskManager {
	return TaskManager{
		me:            m.GetAddress(),
		inventory:     i,
		rosterFactory: roster.NewRosterFactory(m.GetAddressFactory(), s.GetPublicKeyFactory()),
		csFactory:     roster.NewChangeSetFactory(m.GetAddressFactory(), s.GetPublicKeyFactory()),
	}
}

// GetChangeSetFactory implements viewchange.ViewChange.
func (f TaskManager) GetChangeSetFactory() viewchange.ChangeSetFactory {
	return f.csFactory
}

// GetAuthority implements viewchange.ViewChange. It returns the current
// authority based of the last page of the inventory.
func (f TaskManager) GetAuthority(index uint64) (viewchange.Authority, error) {
	page, err := f.inventory.GetPage(index)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read page: %v", err)
	}

	value, err := page.Read(rosterValueKey)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read entry: %v", err)
	}

	authority, ok := value.(viewchange.Authority)
	if !ok {
		return nil, xerrors.Errorf("invalid message type '%T'", value)
	}

	return roster.FromAuthority(authority), nil
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

// Deserialize implements serde.Factory.
func (f TaskManager) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := taskFormats.Get(ctx.GetName())

	ctx = serdeng.WithFactory(ctx, RosterKey{}, f.rosterFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// Register registers the task messages.
func Register(r basic.TransactionFactory, f serdeng.Factory) {
	r.Register(ClientTask{}, f)
	r.Register(ServerTask{}, f)
}
