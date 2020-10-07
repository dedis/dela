package bls

import (
	"fmt"

	"go.dedis.ch/dela/crypto"
)

func ExampleSigner_Sign() {
	signerA := NewSigner()
	signerB := NewSigner()

	publicKeys := []crypto.PublicKey{
		signerA.GetPublicKey(),
		signerB.GetPublicKey(),
	}

	message := []byte("42")

	signatureA, err := signerA.Sign(message)
	if err != nil {
		panic("signer A failed: " + err.Error())
	}

	signatureB, err := signerB.Sign(message)
	if err != nil {
		panic("signer B failed: " + err.Error())
	}

	aggregate, err := signerA.Aggregate(signatureA, signatureB)
	if err != nil {
		panic("aggregate failed: " + err.Error())
	}

	verifier, err := signerA.GetVerifierFactory().FromArray(publicKeys)
	if err != nil {
		panic("verifier failed: " + err.Error())
	}

	err = verifier.Verify(message, aggregate)
	if err != nil {
		panic("invalid signature: " + err.Error())
	}

	fmt.Println("Success")

	// Output: Success
}
