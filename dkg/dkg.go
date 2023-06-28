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
	// where K is the ephemeral DH (Diffie-Hellman) public key
	// and Cs is the resulting, encrypted points
	// or an error if encryption is not possible.
	Encrypt(message []byte) (K kyber.Point, Cs []kyber.Point, err error)

	// Decrypt decrypts a ciphertext (composed of a K and an array of C's)
	// into the original message using the DKG internal private key
	// where K is the ephemeral DH (Diffie-Hellman) public key
	// and Cs is the encrypted points
	Decrypt(K kyber.Point, Cs []kyber.Point) ([]byte, error)

	// Reencrypt reencrypts generate a temporary key from the public key
	// to be able to decrypt the message by the user's private key
	Reencrypt(K kyber.Point, PK kyber.Point) (XhatEnc kyber.Point, err error)

	// Reshare recreates the DKG with an updated list of participants.
	Reshare(co crypto.CollectiveAuthority, newThreshold int) error

	// VerifiableEncrypt encrypts the given message into a ciphertext
	// using the DKG public key and a verifiable encryption function.
	VerifiableEncrypt(message []byte, GBar kyber.Point) (ciphertext types.Ciphertext, remainder []byte, err error)

	// VerifiableDecrypt decrypts a pair of kyber points into the original message
	// using the DKG internal private key and a verifiable encryption function.
	VerifiableDecrypt(ciphertexts []types.Ciphertext) ([][]byte, error)
}
