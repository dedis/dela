package ed25519

import "fmt"

func ExampleSigner_Sign() {
	signerA := NewSigner()

	message := []byte("42")

	signature, err := signerA.Sign(message)
	if err != nil {
		panic("signer failed: " + err.Error())
	}

	err = signerA.GetPublicKey().Verify(message, signature)
	if err != nil {
		panic("invalid signature: " + err.Error())
	}

	fmt.Println("Success")

	// Output: Success
}
