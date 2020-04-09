// Package fake provides fake implementations for interfaces commonly used in
// the repository.
// The implementations offer configuration to return errors when it is needed by
// the unit test and it is also possible to record the call of functions of an
// object in some cases.
package fake

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// Call is a tool to keep track of a function calls.
type Call struct {
	calls [][]interface{}
}

// Get returns the nth call ith parameter.
func (c *Call) Get(n, i int) interface{} {
	return c.calls[n][i]
}

// Len returns the number of calls.
func (c *Call) Len() int {
	return len(c.calls)
}

// Add adds a call to the list.
func (c *Call) Add(args ...interface{}) {
	c.calls = append(c.calls, args)
}

// Address is a fake implementation of mino.Address
type Address struct {
	mino.Address
	index int
}

// Equal implements mino.Address.
func (a Address) Equal(o mino.Address) bool {
	other, ok := o.(Address)
	return ok && other.index == a.index
}

// AddressIterator is a fake implementation of the mino.AddressIterator
// interface.
type AddressIterator struct {
	mino.AddressIterator
	addrs []mino.Address
	index int
}

// HasNext implements mino.AddressIterator.
func (i *AddressIterator) HasNext() bool {
	return i.index+1 < len(i.addrs)
}

// GetNext implements mino.AddressIterator.
func (i *AddressIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.addrs[i.index]
	}
	return nil
}

// PublicKeyIterator is a fake implementation of crypto.PublicKeyIterator.
type PublicKeyIterator struct {
	crypto.PublicKeyIterator
	signers []crypto.AggregateSigner
	index   int
}

// HasNext implements crypto.PublicKeyIterator.
func (i *PublicKeyIterator) HasNext() bool {
	return i.index+1 < len(i.signers)
}

// GetNext implements crypto.PublicKeyIterator.
func (i *PublicKeyIterator) GetNext() crypto.PublicKey {
	if i.HasNext() {
		i.index++
		return i.signers[i.index].GetPublicKey()
	}
	return nil
}

// CollectiveAuthority is a fake implementation of the cosi.CollectiveAuthority
// interface.
type CollectiveAuthority struct {
	crypto.CollectiveAuthority
	addrs   []mino.Address
	signers []crypto.AggregateSigner
}

// GenSigner is a function to generate a signer.
type GenSigner func() crypto.AggregateSigner

// NewAuthority returns a new fake collective authority of size n.
func NewAuthority(n int, g GenSigner) CollectiveAuthority {
	signers := make([]crypto.AggregateSigner, n)
	for i := range signers {
		signers[i] = g()
	}

	addrs := make([]mino.Address, n)
	for i := range addrs {
		addrs[i] = Address{index: i}
	}

	return CollectiveAuthority{
		signers: signers,
		addrs:   addrs,
	}
}

// NewAuthorityFromMino returns a new fake collective authority using
// the addresses of the Mino instances.
func NewAuthorityFromMino(g func() crypto.AggregateSigner, instances ...mino.Mino) CollectiveAuthority {
	signers := make([]crypto.AggregateSigner, len(instances))
	for i := range signers {
		signers[i] = g()
	}

	addrs := make([]mino.Address, len(instances))
	for i, instance := range instances {
		addrs[i] = instance.GetAddress()
	}

	return CollectiveAuthority{
		signers: signers,
		addrs:   addrs,
	}
}

// GetAddress returns the address at the provided index.
func (ca CollectiveAuthority) GetAddress(index int) mino.Address {
	return ca.addrs[index]
}

// GetSigner returns the signer at the provided index.
func (ca CollectiveAuthority) GetSigner(index int) crypto.AggregateSigner {
	return ca.signers[index]
}

// GetPublicKey implements cosi.CollectiveAuthority.
func (ca CollectiveAuthority) GetPublicKey(addr mino.Address) (crypto.PublicKey, int) {
	for i, address := range ca.addrs {
		if address.Equal(addr) {
			return ca.signers[i].GetPublicKey(), i
		}
	}
	return nil, -1
}

// Take implements mino.Players.
func (ca CollectiveAuthority) Take(updaters ...mino.FilterUpdater) mino.Players {
	filter := mino.ApplyFilters(updaters)
	newCA := CollectiveAuthority{
		addrs:   make([]mino.Address, len(filter.Indices)),
		signers: make([]crypto.AggregateSigner, len(filter.Indices)),
	}
	for i, k := range filter.Indices {
		newCA.addrs[i] = ca.addrs[k]
		newCA.signers[i] = ca.signers[k]
	}
	return newCA
}

// Len implements mino.Players.
func (ca CollectiveAuthority) Len() int {
	return len(ca.signers)
}

// AddressIterator implements mino.Players.
func (ca CollectiveAuthority) AddressIterator() mino.AddressIterator {
	return &AddressIterator{addrs: ca.addrs, index: -1}
}

// PublicKeyIterator implements cosi.CollectiveAuthority.
func (ca CollectiveAuthority) PublicKeyIterator() crypto.PublicKeyIterator {
	return &PublicKeyIterator{signers: ca.signers, index: -1}
}

// PublicKeyFactory is a fake implementation of a public key factory.
type PublicKeyFactory struct {
	crypto.PublicKeyFactory
}

// SignatureByte is the byte returned when marshaling a fake signature.
const SignatureByte = 0xfe

// Signature is a fake implementation of the signature.
type Signature struct {
	crypto.Signature
	err error
}

// NewBadSignature returns a signature that will return error when appropriate.
func NewBadSignature() Signature {
	return Signature{err: xerrors.New("fake error")}
}

// Equal implements crypto.Signature.
func (s Signature) Equal(o crypto.Signature) bool {
	_, ok := o.(Signature)
	return ok
}

// Pack implements encoding.Packable.
func (s Signature) Pack(encoding.ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, s.err
}

// MarshalBinary implements crypto.Signature.
func (s Signature) MarshalBinary() ([]byte, error) {
	return []byte{SignatureByte}, s.err
}

// SignatureFactory is a fake implementation of the signature factory.
type SignatureFactory struct {
	crypto.SignatureFactory
	err error
}

// NewBadSignatureFactory returns a signature factory that will return an error
// when appropriate.
func NewBadSignatureFactory() SignatureFactory {
	return SignatureFactory{err: xerrors.New("fake error")}
}

// FromProto implements crypto.SignatureFactory.
func (f SignatureFactory) FromProto(proto.Message) (crypto.Signature, error) {
	return Signature{}, f.err
}

// PublicKey is a fake implementation of crypto.PublicKey.
type PublicKey struct {
	crypto.PublicKey
	err error
}

// NewBadPublicKey returns a new fake public key that returns error when
// appropriate.
func NewBadPublicKey() PublicKey {
	return PublicKey{err: xerrors.New("fake error")}
}

// Verify implements crypto.PublicKey.
func (pk PublicKey) Verify([]byte, crypto.Signature) error {
	return pk.err
}

// Signer is a fake implementation of the crypto.AggregateSigner interface.
type Signer struct {
	crypto.AggregateSigner
	signatureFactory SignatureFactory
	verifierFactory  VerifierFactory
	err              error
}

// NewSigner returns a new instance of the fake signer.
func NewSigner() crypto.AggregateSigner {
	return Signer{}
}

// NewSignerWithSignatureFactory returns a fake signer with the provided
// factory.
func NewSignerWithSignatureFactory(f SignatureFactory) Signer {
	return Signer{signatureFactory: f}
}

// NewSignerWithVerifierFactory returns a new fake signer with the specific
// verifier factory.
func NewSignerWithVerifierFactory(f VerifierFactory) Signer {
	return Signer{verifierFactory: f}
}

// NewBadSigner returns a fake signer that will return an error when
// appropriate.
func NewBadSigner() Signer {
	return Signer{err: xerrors.New("fake error")}
}

// GetPublicKeyFactory implements crypto.Signer.
func (s Signer) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return PublicKeyFactory{}
}

// GetSignatureFactory implements crypto.Signer.
func (s Signer) GetSignatureFactory() crypto.SignatureFactory {
	return s.signatureFactory
}

// GetVerifierFactory implements crypto.Signer.
func (s Signer) GetVerifierFactory() crypto.VerifierFactory {
	return s.verifierFactory
}

// GetPublicKey implements crypto.Signer.
func (s Signer) GetPublicKey() crypto.PublicKey {
	return PublicKey{}
}

// Sign implements crypto.Signer.
func (s Signer) Sign([]byte) (crypto.Signature, error) {
	return Signature{}, s.err
}

// Aggregate implements crypto.AggregateSigner.
func (s Signer) Aggregate(...crypto.Signature) (crypto.Signature, error) {
	return Signature{}, s.err
}

// Verifier is a fake implementation of crypto.Verifier.
type Verifier struct {
	crypto.Verifier
	err error
}

// NewBadVerifier returns a verifier that will return an error when appropriate.
func NewBadVerifier() Verifier {
	return Verifier{err: xerrors.New("fake error")}
}

// Verify implements crypto.Verifier.
func (v Verifier) Verify(msg []byte, s crypto.Signature) error {
	return v.err
}

// VerifierFactory is a fake implementation of crypto.VerifierFactory.
type VerifierFactory struct {
	crypto.VerifierFactory
	verifier Verifier
	err      error
	call     *Call
}

// NewVerifierFactory returns a new fake verifier factory.
func NewVerifierFactory(v Verifier) VerifierFactory {
	return VerifierFactory{verifier: v}
}

// NewVerifierFactoryWithCalls returns a new verifier factory that will register
// the calls.
func NewVerifierFactoryWithCalls(c *Call) VerifierFactory {
	return VerifierFactory{call: c}
}

// NewBadVerifierFactory returns a fake verifier factory that returns an error
// when appropriate.
func NewBadVerifierFactory() VerifierFactory {
	return VerifierFactory{err: xerrors.New("fake error")}
}

// FromAuthority implements crypto.VerifierFactory.
func (f VerifierFactory) FromAuthority(ca crypto.CollectiveAuthority) (crypto.Verifier, error) {
	if f.call != nil {
		f.call.Add(ca)
	}
	return f.verifier, f.err
}

// BadPackEncoder is a fake implementation of encoding.ProtoMarshaler.
type BadPackEncoder struct {
	encoding.ProtoEncoder
}

// Pack implements encoding.ProtoMarshaler.
func (e BadPackEncoder) Pack(encoding.Packable) (proto.Message, error) {
	return nil, xerrors.New("fake error")
}

// BadPackAnyEncoder is a fake implementation of encoding.ProtoMarshaler.
type BadPackAnyEncoder struct {
	encoding.ProtoEncoder
}

// PackAny implements encoding.ProtoMarshaler.
func (e BadPackAnyEncoder) PackAny(encoding.Packable) (*any.Any, error) {
	return nil, xerrors.New("fake error")
}

// BadUnmarshalAnyEncoder is a fake implementation of encoding.ProtoMarshaler.
type BadUnmarshalAnyEncoder struct {
	encoding.ProtoEncoder
}

// UnmarshalAny implements encoding.ProtoMarshaler.
func (e BadUnmarshalAnyEncoder) UnmarshalAny(*any.Any, proto.Message) error {
	return xerrors.New("fake error")
}

// Mino is a fake implementation of mino.Mino.
type Mino struct {
	mino.Mino
	err error
}

// NewBadMino returns a Mino instance that returns an error when appropriate.
func NewBadMino() Mino {
	return Mino{err: xerrors.New("fake error")}
}

// MakeRPC implements mino.Mino.
func (m Mino) MakeRPC(string, mino.Handler) (mino.RPC, error) {
	return nil, m.err
}
