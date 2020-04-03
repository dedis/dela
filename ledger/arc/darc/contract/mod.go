package contract

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/consumer"
	sc "go.dedis.ch/fabric/ledger/consumer/smartcontract"
	"golang.org/x/xerrors"
)

// ContractName is the name used to differentiate the contract from others.
const ContractName = "darc"

// Contract is the smart contract implementation for DARCs.
type Contract struct {
	factory darc.Factory
}

// Spawn implements smartcontract.Contract. It returns an access control if the
// transaction is correct.
func (c Contract) Spawn(ctx sc.SpawnContext) (proto.Message, []byte, error) {
	gnrc, err := c.factory.FromProto(ctx.GetAction().Argument)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't decode argument: %v", err)
	}

	access, ok := gnrc.(darc.Access)
	if !ok {
		return nil, nil, xerrors.Errorf("invalid access type '%T'", gnrc)
	}

	// Set a rule to allow the creator of the DARC to update it.
	rule := arc.Compile(ContractName, "invoke")
	access, err = access.Evolve(rule, ctx.GetTransaction().GetIdentity())
	if err != nil {
		return nil, nil, err
	}

	fabric.Logger.Trace().Msgf("new darc: %+v", access)

	darcpb, err := access.Pack(nil)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't pack darc: %v", err)
	}

	return darcpb, ctx.GetTransaction().GetID(), nil
}

// Invoke implements smartcontract.Contract.
func (c Contract) Invoke(ctx sc.InvokeContext) (proto.Message, error) {
	instance, err := ctx.Read(ctx.GetAction().Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read instance: %v", err)
	}

	access, err := c.factory.FromProto(instance.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode darc: %v", err)
	}

	// TODO: use argument to update the darc..

	darcpb, err := access.(darc.Access).Pack(nil)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack darc: %v", err)
	}

	return darcpb, nil
}

// NewGenesisTransaction returns a transaction to spawn a new empty DARC.
func NewGenesisTransaction(factory sc.TransactionFactory) (consumer.Transaction, error) {
	spawn := sc.SpawnAction{
		ContractID: ContractName,
		Argument:   &darc.AccessControlProto{},
	}

	tx, err := factory.New(spawn)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// NewUpdateTransaction returns a transaction to update an existing DARC.
func NewUpdateTransaction(factory sc.TransactionFactory, key []byte) (consumer.Transaction, error) {
	invoke := sc.InvokeAction{
		Key:      key,
		Argument: &empty.Empty{},
	}

	tx, err := factory.New(invoke)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// RegisterContract can be used to enable DARC for a smart contract consumer.
func RegisterContract(c sc.Consumer) {
	c.Register(ContractName, Contract{})
}
