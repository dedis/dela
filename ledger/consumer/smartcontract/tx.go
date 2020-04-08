package smartcontract

import (
	"encoding/binary"
	fmt "fmt"
	"hash"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/common"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/ledger/arc"
	"go.dedis.ch/fabric/ledger/consumer"
	"golang.org/x/xerrors"
)

type action interface {
	encoding.Packable

	hashTo(hash.Hash, encoding.ProtoMarshaler) error
}

// transaction is an atomic execution.
//
// - implements ledger.transaction
type transaction struct {
	hash      []byte
	nonce     uint64
	action    action
	identity  crypto.PublicKey
	signature crypto.Signature
}

// GetID implements ledger.Transaction. It returns the unique identifier of the
// transaction.
func (t transaction) GetID() []byte {
	return t.hash[:]
}

func (t transaction) GetIdentity() arc.Identity {
	return t.identity
}

func (t transaction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &TransactionProto{
		Nonce: t.nonce,
	}

	var err error
	pb.Identity, err = enc.PackAny(t.identity)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack identity: %v", err)
	}

	pb.Signature, err = enc.PackAny(t.signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack signature: %v", err)
	}

	actionpb, err := enc.Pack(t.action)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack action: %v", err)
	}

	switch action := actionpb.(type) {
	case *Spawn:
		pb.Action = &TransactionProto_Spawn{Spawn: action}
	case *Invoke:
		pb.Action = &TransactionProto_Invoke{Invoke: action}
	case *Delete:
		pb.Action = &TransactionProto_Delete{Delete: action}
	}

	return pb, nil
}

func (t transaction) String() string {
	return fmt.Sprintf("Transaction[%v]", t.identity)
}

func (t transaction) computeHash(h hash.Hash, enc encoding.ProtoMarshaler) ([]byte, error) {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer[:], t.nonce)

	_, err := h.Write(buffer)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write nonce: %v", err)
	}

	buffer, err = t.identity.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal identity: %v", err)
	}

	_, err = h.Write(buffer)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write identity: %v", err)
	}

	err = t.action.hashTo(h, enc)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write action: %v", err)
	}

	return h.Sum(nil), nil
}

// SpawnAction is a transaction action that will create a new instance.
//
// - implements encoding.Packable
type SpawnAction struct {
	ContractID string
	Argument   proto.Message
}

// Pack implements encoding.Packable. It returns the protobuf message for a
// spawn transaction.
func (t SpawnAction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Spawn{
		ContractID: t.ContractID,
	}

	if t.Argument != nil {
		var err error
		pb.Argument, err = enc.MarshalAny(t.Argument)
		if err != nil {
			return nil, xerrors.Errorf("couldn't marshal the argument: %v", err)
		}
	}

	return pb, nil
}

func (t SpawnAction) hashTo(h hash.Hash, enc encoding.ProtoMarshaler) error {
	_, err := h.Write([]byte(t.ContractID))
	if err != nil {
		return xerrors.Errorf("couldn't write contract ID: %v", err)
	}

	if t.Argument != nil {
		err = enc.MarshalStable(h, t.Argument)
		if err != nil {
			return xerrors.Errorf("couldn't write argument: %v", err)
		}
	}

	return nil
}

// InvokeAction is a transaction action that will update an instance.
//
// - implements encoding.Packable
type InvokeAction struct {
	Key      []byte
	Argument proto.Message
}

// Pack implements encoding.Packable. It returns the protobuf message of the
// invoke transaction.
func (t InvokeAction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	argany, err := enc.MarshalAny(t.Argument)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal the argument: %v", err)
	}

	pb := &Invoke{
		Key:      t.Key,
		Argument: argany,
	}

	return pb, nil
}

func (t InvokeAction) hashTo(h hash.Hash, enc encoding.ProtoMarshaler) error {
	_, err := h.Write(t.Key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	if t.Argument != nil {
		err = enc.MarshalStable(h, t.Argument)
		if err != nil {
			return xerrors.Errorf("couldn't write argument: %v", err)
		}
	}

	return nil
}

// DeleteAction is a transaction action that will tag an instance as deleted so
// that it will become immutable.
//
// implements encoding.Packable
type DeleteAction struct {
	Key []byte
}

// Pack implements encoding.Packable. It returns the protobuf message for the
// delete transaction.
func (t DeleteAction) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &Delete{
		Key: t.Key,
	}

	return pb, nil
}

func (t DeleteAction) hashTo(h hash.Hash, enc encoding.ProtoMarshaler) error {
	_, err := h.Write(t.Key)
	if err != nil {
		return xerrors.Errorf("couldn't write key: %v", err)
	}

	return nil
}

// TransactionFactory is an implementation of a Byzcoin transaction factory.
//
// - implements ledger.TransactionFactory
type TransactionFactory struct {
	signer           crypto.Signer
	hashFactory      crypto.HashFactory
	publicKeyFactory crypto.PublicKeyFactory
	signatureFactory crypto.SignatureFactory
	encoder          encoding.ProtoMarshaler
}

// NewTransactionFactory returns a new instance of the transaction factory.
//
// - implements ledger.TransactionFactory
func NewTransactionFactory(signer crypto.Signer) TransactionFactory {
	return TransactionFactory{
		signer:           signer,
		hashFactory:      crypto.NewSha256Factory(),
		publicKeyFactory: common.NewPublicKeyFactory(),
		signatureFactory: common.NewSignatureFactory(),
		encoder:          encoding.NewProtoEncoder(),
	}
}

// New returns a new transaction.
func (f TransactionFactory) New(action action) (consumer.Transaction, error) {
	tx := transaction{
		nonce:    0, // TODO:
		identity: f.signer.GetPublicKey(),
		action:   action,
	}

	var err error
	tx.hash, err = tx.computeHash(f.hashFactory.New(), f.encoder)
	if err != nil {
		return tx, xerrors.Errorf("couldn't compute hash: %v", err)
	}

	tx.signature, err = f.signer.Sign(tx.hash)
	if err != nil {
		return tx, xerrors.Errorf("couldn't sign tx: %v", err)
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

	tx := transaction{
		nonce: txProto.GetNonce(),
	}

	spawn := txProto.GetSpawn()
	if spawn != nil {
		arg, err := f.encoder.UnmarshalDynamicAny(spawn.GetArgument())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal argument: %v", err)
		}

		tx.action = SpawnAction{
			ContractID: spawn.GetContractID(),
			Argument:   arg,
		}
	}

	invoke := txProto.GetInvoke()
	if invoke != nil {
		arg, err := f.encoder.UnmarshalDynamicAny(invoke.GetArgument())
		if err != nil {
			return nil, xerrors.Errorf("couldn't unmarshal argument: %v", err)
		}

		tx.action = InvokeAction{
			Key:      invoke.GetKey(),
			Argument: arg,
		}
	}

	delete := txProto.GetDelete()
	if delete != nil {
		tx.action = DeleteAction{
			Key: delete.GetKey(),
		}
	}

	err := f.fillIdentity(&tx, txProto)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (f TransactionFactory) fillIdentity(tx *transaction, pb *TransactionProto) error {
	var err error
	tx.identity, err = f.publicKeyFactory.FromProto(pb.GetIdentity())
	if err != nil {
		return xerrors.Errorf("couldn't decode public key: %v", err)
	}

	tx.signature, err = f.signatureFactory.FromProto(pb.GetSignature())
	if err != nil {
		return xerrors.Errorf("couldn't decode signature: %v", err)
	}

	tx.hash, err = tx.computeHash(f.hashFactory.New(), f.encoder)
	if err != nil {
		return xerrors.Errorf("couldn't compute hash: %v", err)
	}

	err = tx.identity.Verify(tx.hash, tx.signature)
	if err != nil {
		return xerrors.Errorf("signature does not match tx: %v", err)
	}

	return nil
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

// GetArcID implements consumer.Instance. It returns the access control
// identifier for this instance.
func (i contractInstance) GetArcID() []byte {
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
