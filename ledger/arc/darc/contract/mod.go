package contract

import (
	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/arc/darc"
	sc "go.dedis.ch/fabric/ledger/consumer/smartcontract"
	"golang.org/x/xerrors"
)

// ContractName is the name used to differentiate the contract from others.
const ContractName = "darc"

// Contract is the smart contract implementation for DARCs.
type Contract struct {
	encoder encoding.ProtoMarshaler
	factory arc.AccessControlFactory
}

// Spawn implements smartcontract.Contract. It returns an access control if the
// transaction is correct.
func (c Contract) Spawn(ctx sc.SpawnContext) (proto.Message, []byte, error) {
	gnrc, err := c.factory.FromProto(ctx.GetAction().Argument)
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't decode argument: %v", err)
	}

	access, ok := gnrc.(darc.EvolvableAccessControl)
	if !ok {
		return nil, nil,
			xerrors.Errorf("'%T' does not implement 'darc.EvolvableAccessControl'", gnrc)
	}

	// Set a rule to allow the creator of the DARC to update it.
	rule := arc.Compile(ContractName, "invoke")
	access, err = access.Evolve(rule, ctx.GetTransaction().GetIdentity())
	if err != nil {
		return nil, nil, xerrors.Errorf("couldn't evolve darc: %v", err)
	}

	darcpb, err := c.encoder.Pack(access)
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

	gnrc, err := c.factory.FromProto(instance.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode darc: %v", err)
	}

	access, ok := gnrc.(darc.EvolvableAccessControl)
	if !ok {
		return nil, xerrors.Errorf("'%T' does not implement 'darc.EvolvableAccessControl'", gnrc)
	}

	// TODO: use argument to update the darc..

	darcpb, err := c.encoder.Pack(access)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack darc: %v", err)
	}

	return darcpb, nil
}

// NewGenesisAction returns a transaction to spawn a new empty DARC.
func NewGenesisAction() sc.SpawnAction {
	return sc.SpawnAction{
		ContractID: ContractName,
		Argument:   &darc.AccessControlProto{},
	}
}

// NewUpdateAction returns a transaction to update an existing DARC.
func NewUpdateAction(key []byte) sc.InvokeAction {
	return sc.InvokeAction{
		Key:      key,
		Argument: &empty.Empty{},
	}
}

// RegisterContract can be used to enable DARC for a smart contract consumer.
func RegisterContract(c sc.Consumer) {
	c.Register(ContractName, Contract{
		encoder: encoding.NewProtoEncoder(),
		factory: darc.NewFactory(),
	})
}
