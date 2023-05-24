package dkg

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
)

// DKG defines the primitive to start a DKG protocol
type DKG interface {
	// Listen starts the RPC. This function should be called on each node that
	// wishes to participate in a DKG.
	Listen() (Actor, error)
}

// Actor defines the primitives to use a DKG protocol
type Actor interface {
	// Setup must be first called by ONE of the actor to use the subsequent
	// functions. It creates the public distributed key and the private share on
	// each node. Each node represented by a player must first execute Listen().
	Setup(co crypto.CollectiveAuthority, threshold int) (pubKey kyber.Point, err error)

	// GetPublicKey returns the collective public key. Returns an error it the
	// setup has not been done.
	GetPublicKey() (kyber.Point, error)

	Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error)
	Decrypt(K, C kyber.Point) ([]byte, error)

	Reshare(co crypto.CollectiveAuthority, newThreshold int) error

	VerifiableEncrypt(message []byte, GBar kyber.Point) (ciphertext types.Ciphertext, remainder []byte, err error)
	VerifiableDecrypt(ciphertexts []types.Ciphertext) ([][]byte, error)

	EncryptSecret(message []byte) (U kyber.Point, Cs []kyber.Point)
	ReencryptSecret(U kyber.Point, Pk kyber.Point, threshold int) (Uis []*share.PubShare, err error)

	DecryptSecret(Cs []kyber.Point, XhatEnc kyber.Point, Sk kyber.Scalar) (message []byte, err error)
}
