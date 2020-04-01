package smartcontract

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"golang.org/x/xerrors"
)

// transaction is an atomic execution.
//
// - implements ledger.transaction
type transaction struct {
	hash     []byte
	nonce    uint64
	identity []byte
}

// GetID implements ledger.Transaction. It returns the unique identifier of the
// transaction.
func (t transaction) GetID() []byte {
	return t.hash[:]
}

// SpawnTransaction is a smart contract transaction that will create a new
// instance.
//
// - implements encoding.Packable
type SpawnTransaction struct {
	transaction
	ContractID string
	Argument   proto.Message
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// spawn transaction.
func (t SpawnTransaction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	argany, err := enc.MarshalAny(t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the argument: %v", err)
	}

	tx := &TransactionProto{
		Identity: t.identity,
		Nonce:    0,
		Action: &TransactionProto_Spawn{
			Spawn: &Spawn{
				ContractID: t.ContractID,
				Argument:   argany,
			},
		},
	}

	return tx, nil
}

func (t SpawnTransaction) computeHash(f crypto.HashFactory,
	enc encoding.ProtoMarshaler) ([]byte, error) {

	h := f.New()
	_, err := h.Write([]byte(t.ContractID))
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the contract ID: %v", err)
	}

	err = enc.MarshalStable(h, t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the argument: %v", err)
	}

	return h.Sum(nil), nil
}

// InvokeTransaction is a smart contract transaction that will update an
// instance.
//
// - implements encoding.Packable
type InvokeTransaction struct {
	transaction
	Key      []byte
	Argument proto.Message
}

// Pack implements encoding.Packable. It returns the protobuf message of the
// invoke transaction.
func (t InvokeTransaction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	argany, err := enc.MarshalAny(t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the argument: %v", err)
	}

	tx := &TransactionProto{
		Identity: t.identity,
		Nonce:    0,
		Action: &TransactionProto_Invoke{
			Invoke: &Invoke{
				Key:      t.Key,
				Argument: argany,
			},
		},
	}

	return tx, nil
}

func (t InvokeTransaction) computeHash(f crypto.HashFactory,
	enc encoding.ProtoMarshaler) ([]byte, error) {

	h := f.New()
	_, err := h.Write(t.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the key: %v", err)
	}

	err = enc.MarshalStable(h, t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the argument: %v", err)
	}

	return h.Sum(nil), nil
}

// DeleteTransaction is a smart contract transaction that will tag an instance
// as deleted so that it will become immutable.
//
// implements encoding.Packable
type DeleteTransaction struct {
	transaction
	Key []byte
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// delete transaction.
func (t DeleteTransaction) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &TransactionProto{
		Identity: t.identity,
		Nonce:    0,
		Action: &TransactionProto_Delete{
			Delete: &Delete{Key: t.Key},
		},
	}

	return pb, nil
}

func (t DeleteTransaction) computeHash(f crypto.HashFactory) ([]byte, error) {
	h := f.New()

	_, err := h.Write(t.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the key: %v", err)
	}

	return h.Sum(nil), nil
}

// TransactionFactory is an implementation of a Byzcoin transaction factory.
//
// - implements ledger.TransactionFactory
type TransactionFactory struct {
	identity    []byte // TODO: signed identity
	hashFactory crypto.HashFactory
	encoder     encoding.ProtoMarshaler
}

// NewTransactionFactory returns a new instance of the transaction factory.
func NewTransactionFactory(identity []byte) TransactionFactory {
	return TransactionFactory{
		identity:    identity,
		hashFactory: crypto.NewSha256Factory(),
		encoder:     encoding.NewProtoEncoder(),
	}
}

// NewSpawn returns a new spawn transaction.
func (f TransactionFactory) NewSpawn(contractID string,
	arg proto.Message) (SpawnTransaction, error) {

	if arg == nil {
		return SpawnTransaction{}, xerrors.New("argument cannot be nil")
	}

	tx := SpawnTransaction{
		ContractID: contractID,
		Argument:   arg,
	}

	hash, err := tx.computeHash(f.hashFactory, f.encoder)
	if err != nil {
		return tx, xerrors.Errorf("couldn't hash tx: %v", err)
	}

	tx.transaction = transaction{
		hash:     hash,
		identity: f.identity,
	}

	return tx, nil
}

// NewInvoke returns a new invoke transaction.
func (f TransactionFactory) NewInvoke(key []byte, arg proto.Message) (InvokeTransaction, error) {
	if arg == nil {
		return InvokeTransaction{}, xerrors.New("argument cannot be nil")
	}

	tx := InvokeTransaction{
		Key:      key,
		Argument: arg,
	}

	hash, err := tx.computeHash(f.hashFactory, f.encoder)
	if err != nil {
		return tx, xerrors.Errorf("couldn't hash tx: %v", err)
	}

	tx.transaction = transaction{
		hash:     hash,
		identity: f.identity,
	}

	return tx, nil
}

// NewDelete returns a new delete transaction.
func (f TransactionFactory) NewDelete(key []byte) (DeleteTransaction, error) {
	tx := DeleteTransaction{
		Key: key,
	}

	hash, err := tx.computeHash(f.hashFactory)
	if err != nil {
		return tx, xerrors.Errorf("couldn't hash tx: %v", err)
	}

	tx.transaction = transaction{
		hash:     hash,
		identity: f.identity,
	}

	return tx, nil
}

// FromProto implements ledger.TransactionFactory. It returns a new transaction
// built from the protobuf message.
func (f TransactionFactory) FromProto(pb proto.Message) (consumer.Transaction, error) {
	var txProto *TransactionProto

	switch in := pb.(type) {
	case *any.Any:
		txProto = &TransactionProto{}
		err := f.encoder.UnmarshalAny(in, txProto)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal input: %v", err)
		}
	case *TransactionProto:
		txProto = in
	default:
		return nil, xerrors.Errorf("invalid transaction type '%T'", pb)
	}

	if spawn := txProto.GetSpawn(); spawn != nil {
		arg, err := f.encoder.UnmarshalDynamicAny(spawn.GetArgument())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal argument: %v", err)
		}

		tx, err := f.NewSpawn(spawn.GetContractID(), arg)
		if err != nil {
			return nil, err
		}

		tx.identity = txProto.GetIdentity()
		return tx, nil
	}

	if invoke := txProto.GetInvoke(); invoke != nil {
		arg, err := f.encoder.UnmarshalDynamicAny(invoke.GetArgument())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal argument: %v", err)
		}

		tx, err := f.NewInvoke(invoke.GetKey(), arg)
		if err != nil {
			return nil, err
		}

		tx.identity = txProto.GetIdentity()
		return tx, nil
	}

	if delete := txProto.GetDelete(); delete != nil {
		tx, err := f.NewDelete(delete.GetKey())
		if err != nil {
			return nil, err
		}

		tx.identity = txProto.GetIdentity()
		return tx, nil
	}

	return nil, xerrors.New("tx is malformed")
}

// ContractInstance is a specialization of the consumer instance to include
// smart contract details.
type ContractInstance interface {
	consumer.Instance

	GetContractID() string
	Deleted() bool
}

// contractInstance is the result of a smart contract transaction execution. The
// key is defined by the hash of the spawn transaction that created the
// instance. It is immutable exactly like the the contract identifier.
//
// - implements consumer.Instance
// - implements smartcontract.ContractInstance
// - implements encoding.Packable
type contractInstance struct {
	key           []byte
	accessControl []byte
	contractID    string
	deleted       bool
	value         proto.Message
}

// GetKey implements consumer.Instance. It returns the key of the instance.
func (i contractInstance) GetKey() []byte {
	return i.key
}

func (i contractInstance) GetAccessControlID() []byte {
	return i.accessControl
}

// GetContractID implements smartcontract.ContractInstance. It returns the
// contract identifier.
func (i contractInstance) GetContractID() string {
	return i.contractID
}

// GetValue implements consumer.Instance. It returns the value produced by the
// transaction execution.
func (i contractInstance) GetValue() proto.Message {
	return i.value
}

// Deleted implements smartcontract.ContractInstance. It returns true if the
// instance has been permanently deleted.
func (i contractInstance) Deleted() bool {
	return i.deleted
}

// Pack implements encoding.Packable. It returns the protobuf message for this
// instance.
func (i contractInstance) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &InstanceProto{
		Key:           i.key,
		ContractID:    i.contractID,
		Deleted:       i.deleted,
		AccessControl: i.accessControl,
	}

	var err error
	pb.Value, err = enc.MarshalAny(i.value)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the value: %v", err)
	}

	return pb, nil
}

// instanceFactory is the implementation of the consumer.InstanceFactory for
// smart contract instances.
//
// - implements consumer.InstanceFactory
type instanceFactory struct {
	encoder encoding.ProtoMarshaler
}

// FromProto implements consumer.InstanceFactory. It returns the instance from
// the protobuf message if it applies, otherwise an error.
func (f instanceFactory) FromProto(pb proto.Message) (consumer.Instance, error) {
	var instancepb *InstanceProto
	switch i := pb.(type) {
	case *any.Any:
		instancepb = &InstanceProto{}
		err := f.encoder.UnmarshalAny(i, instancepb)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal: %v", err)
		}
	case *InstanceProto:
		instancepb = i
	default:
		return nil, xerrors.Errorf("invalid instance type '%T'", pb)
	}

	value, err := f.encoder.UnmarshalDynamicAny(instancepb.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal the value: %v", err)
	}

	instance := contractInstance{
		key:           instancepb.GetKey(),
		accessControl: instancepb.GetAccessControl(),
		contractID:    instancepb.GetContractID(),
		deleted:       instancepb.GetDeleted(),
		value:         value,
	}

	return instance, nil
}
