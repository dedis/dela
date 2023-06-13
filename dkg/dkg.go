package dkg

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/kyber/v3"
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

	// Encrypt encrypts the given message into kyber points
	// using the DKG public key
	Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error)
	// Decrypt decrypts a pair of kyber points into the original message
	// using the DKG internal private key
	Decrypt(K, C kyber.Point) ([]byte, error)

	// Reshare recreates the DKG with an updated list of participants.
	Reshare(co crypto.CollectiveAuthority, newThreshold int) error

	// VerifiableEncrypt encrypts the given message into kyber points
	// using the DKG public key and proof of work algorithm.
	VerifiableEncrypt(message []byte, GBar kyber.Point) (ciphertext types.Ciphertext, remainder []byte, err error)

	// VerifiableDecrypt decrypts a pair of kyber points into the original message
	// using the DKG internal private key and a proof of work algorithm.
	VerifiableDecrypt(ciphertexts []types.Ciphertext) ([][]byte, error)

	// EncryptSecret encrypts the given message the calypso way
	EncryptSecret(message []byte) (U kyber.Point, Cs []kyber.Point)

	// ReencryptSecret is the first version of the calypso
	// reencryption algorithm in DELA
	ReencryptSecret(U kyber.Point, Pk kyber.Point) (XhatEnc kyber.Point, err error)

	// DecryptSecret is a helper function to decrypt a secret message previously
	// encrypted and reencrypted by the DKG
	DecryptSecret(Cs []kyber.Point, XhatEnc kyber.Point, Sk kyber.Scalar) (message []byte, err error)
}
