package common_test

import (
	"fmt"

	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/common"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/serde/json"
)

func ExamplePublicKeyFactory_PublicKeyOf_bls() {
	// BLS is already registered by default
	factory := common.NewPublicKeyFactory()

	ctx := json.NewContext()

	message := []byte("42")

	signer := bls.NewSigner()
	publicKey := signer.GetPublicKey()

	signature, err := signer.Sign(message)
	if err != nil {
		panic("signing failed: " + err.Error())
	}

	data, err := publicKey.Serialize(ctx)
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	// Transmit the data over a physical communication channel...

	result, err := factory.PublicKeyOf(ctx, data)
	if err != nil {
		panic("factory failed: " + err.Error())
	}

	err = result.Verify(message, signature)
	if err != nil {
		fmt.Println("public key is invalid")
	} else {
		fmt.Println("signature is verified")
	}

	// Output: signature is verified
}

func ExamplePublicKeyFactory_PublicKeyOf_ed25519() {
	factory := common.NewPublicKeyFactory()
	factory.RegisterAlgorithm(ed25519.Algorithm, ed25519.NewPublicKeyFactory())

	ctx := json.NewContext()

	message := []byte("42")

	signer := ed25519.NewSigner()
	publicKey := signer.GetPublicKey()

	signature, err := signer.Sign(message)
	if err != nil {
		panic("signature failed: " + err.Error())
	}

	data, err := publicKey.Serialize(ctx)
	if err != nil {
		panic("serialization failed: " + err.Error())
	}

	// Transmit the data over a physical communication channel...

	result, err := factory.PublicKeyOf(ctx, data)
	if err != nil {
		panic("factory failed: " + err.Error())
	}

	err = result.Verify(message, signature)
	if err != nil {
		fmt.Println("public key is invalid")
	} else {
		fmt.Println("signature is verified")
	}

	// Output: signature is verified
}
