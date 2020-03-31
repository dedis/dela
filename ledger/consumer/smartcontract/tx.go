package smartcontract

import (
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/inventory"
	"golang.org/x/xerrors"
)

// transaction is an atomic execution.
//
// - implements ledger.transaction
type transaction struct {
	hash []byte
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
	encoder    encoding.ProtoMarshaler
	ContractID string
	Argument   proto.Message
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// spawn transaction.
func (t SpawnTransaction) Pack() (proto.Message, error) {
	argany, err := t.encoder.MarshalAny(t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the argument: %v", err)
	}

	tx := &SpawnTransactionProto{
		ContractID: t.ContractID,
		Argument:   argany,
	}

	return tx, nil
}

func (t SpawnTransaction) computeHash(f crypto.HashFactory,
	m encoding.JSONMarshaler) ([]byte, error) {

	h := f.New()
	_, err := h.Write([]byte(t.ContractID))
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the contract ID: %v", err)
	}

	err = m.Marshal(h, t.Argument)
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
	encoder  encoding.ProtoMarshaler
	Key      []byte
	Argument proto.Message
}

// Pack implements encoding.Packable. It returns the protobuf message of the
// invoke transaction.
func (t InvokeTransaction) Pack() (proto.Message, error) {
	argany, err := t.encoder.MarshalAny(t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the argument: %v", err)
	}

	tx := &InvokeTransactionProto{
		Key:      t.Key,
		Argument: argany,
	}

	return tx, nil
}

func (t InvokeTransaction) computeHash(f crypto.HashFactory,
	m encoding.JSONMarshaler) ([]byte, error) {

	h := f.New()
	_, err := h.Write(t.Key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write the key: %v", err)
	}

	err = m.Marshal(h, t.Argument)
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
func (t DeleteTransaction) Pack() (proto.Message, error) {
	return &DeleteTransactionProto{Key: t.Key}, nil
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
	hashFactory crypto.HashFactory
	encoder     encoding.ProtoMarshaler
	marshaler   encoding.JSONMarshaler
}

// NewTransactionFactory returns a new instance of the transaction factory.
func NewTransactionFactory() TransactionFactory {
	return TransactionFactory{
		hashFactory: crypto.NewSha256Factory(),
		encoder:     encoding.NewProtoEncoder(),
		marshaler:   &jsonpb.Marshaler{},
	}
}

// NewSpawn returns a new spawn transaction.
func (f TransactionFactory) NewSpawn(contractID string,
	arg proto.Message) (SpawnTransaction, error) {

	if arg == nil {
		return SpawnTransaction{}, xerrors.New("argument cannot be nil")
	}

	tx := SpawnTransaction{
		encoder:    f.encoder,
		ContractID: contractID,
		Argument:   arg,
	}

	hash, err := tx.computeHash(f.hashFactory, f.marshaler)
	if err != nil {
		return tx, xerrors.Errorf("couldn't hash tx: %v", err)
	}

	tx.transaction = transaction{hash: hash}

	return tx, nil
}

// NewInvoke returns a new invoke transaction.
func (f TransactionFactory) NewInvoke(key []byte, arg proto.Message) (InvokeTransaction, error) {
	if arg == nil {
		return InvokeTransaction{}, xerrors.New("argument cannot be nil")
	}

	tx := InvokeTransaction{
		encoder:  f.encoder,
		Key:      key,
		Argument: arg,
	}

	hash, err := tx.computeHash(f.hashFactory, f.marshaler)
	if err != nil {
		return tx, xerrors.Errorf("couldn't hash tx: %v", err)
	}

	tx.transaction = transaction{hash: hash}

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

	tx.transaction = transaction{hash: hash}

	return tx, nil
}

// FromProto implements ledger.TransactionFactory. It returns a new transaction
// built from the protobuf message.
func (f TransactionFactory) FromProto(pb proto.Message) (consumer.Transaction, error) {
	var txProto proto.Message

	switch in := pb.(type) {
	case *any.Any:
		var err error
		txProto, err = f.encoder.UnmarshalDynamicAny(in)
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal input: %v", err)
		}
	default:
		txProto = pb
	}

	switch tx := txProto.(type) {
	case *SpawnTransactionProto:
		arg, err := f.encoder.UnmarshalDynamicAny(tx.GetArgument())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal argument: %v", err)
		}

		return f.NewSpawn(tx.GetContractID(), arg)
	case *InvokeTransactionProto:
		arg, err := f.encoder.UnmarshalDynamicAny(tx.GetArgument())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal argument: %v", err)
		}

		return f.NewInvoke(tx.GetKey(), arg)
	case *DeleteTransactionProto:
		return f.NewDelete(tx.GetKey())
	default:
		return nil, xerrors.Errorf("invalid transaction type '%T'", txProto)
	}
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
	key        []byte
	contractID string
	deleted    bool
	value      proto.Message

	encoder encoding.ProtoMarshaler
}

// GetKey implements consumer.Instance. It returns the key of the instance.
func (i contractInstance) GetKey() []byte {
	return i.key
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
func (i contractInstance) Pack() (proto.Message, error) {
	pb := &InstanceProto{
		Key:        i.key,
		ContractID: i.contractID,
		Deleted:    i.deleted,
	}

	var err error
	pb.Value, err = i.encoder.MarshalAny(i.value)
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
			return nil, encoding.NewAnyDecodingError(instancepb, err)
		}
	case *InstanceProto:
		instancepb = i
	}

	value, err := f.encoder.UnmarshalDynamicAny(instancepb.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal the value: %v", err)
	}

	instance := contractInstance{
		key:        instancepb.GetKey(),
		contractID: instancepb.GetContractID(),
		deleted:    instancepb.GetDeleted(),
		value:      value,
		encoder:    f.encoder,
	}

	return instance, nil
}

// transactionContext is provided to smart contract execution. It provides
// valuable information to the implementation.
type transactionContext struct {
	encoder encoding.ProtoMarshaler
	page    inventory.Page
}

func (ctx transactionContext) Read(key []byte) (ContractInstance, error) {
	entry, err := ctx.page.Read(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read the entry: %v", err)
	}

	instancepb, ok := entry.(*InstanceProto)
	if !ok {
		return nil, xerrors.Errorf("instance type '%T' != '%T'", entry, instancepb)
	}

	value, err := ctx.encoder.UnmarshalDynamicAny(instancepb.GetValue())
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal the value: %v", err)
	}

	instance := contractInstance{
		key:        key,
		value:      value,
		contractID: instancepb.GetContractID(),
		deleted:    instancepb.GetDeleted(),
		encoder:    ctx.encoder,
	}

	return instance, nil
}

// SpawnContext is the context provided to a smart contract execution of a spawn
// transaction.
type SpawnContext struct {
	transactionContext

	Transaction SpawnTransaction
}

// InvokeContext is the context provided to a smart contract execution of an
// invoke transaction.
type InvokeContext struct {
	transactionContext

	Transaction InvokeTransaction
}
