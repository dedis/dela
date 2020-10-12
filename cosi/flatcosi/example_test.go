package flatcosi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
)

func Example() {
	// Create the network overlay instances. It uses channels to communicate.
	manager := minoch.NewManager()

	mA := minoch.MustCreate(manager, "A")
	mB := minoch.MustCreate(manager, "B")

	// The list of participants to the signature.
	roster := fake.NewAuthorityFromMino(bls.Generate, mA, mB)

	// Create the collective signing endpoints for both A and B.
	cosiA := NewFlat(mA, roster.GetSigner(0).(crypto.AggregateSigner))

	actor, err := cosiA.Listen(exampleReactor{})
	if err != nil {
		panic(fmt.Sprintf("failed to listen on root: %+v", err))
	}

	cosiB := NewFlat(mB, roster.GetSigner(1).(crypto.AggregateSigner))
	_, err = cosiB.Listen(exampleReactor{})
	if err != nil {
		panic(fmt.Sprintf("failed to listen on child: %+v", err))
	}

	// Context to timeout after 30 seconds if no signature is received.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg := exampleMessage{Value: "42"}

	signature, err := actor.Sign(ctx, msg, roster)
	if err != nil {
		panic(fmt.Sprintf("failed to sign: %+v", err))
	}

	// We need a verifier implementation to support collective signatures.
	verifier, err := cosiA.GetVerifierFactory().FromAuthority(roster)
	if err != nil {
		panic(fmt.Sprintf("verifier failed: %+v", err))
	}

	err = verifier.Verify([]byte(msg.Value), signature)
	if err != nil {
		panic(fmt.Sprintf("signature is invalid: %+v", err))
	}

	fmt.Println("Success", err == nil)

	// Output: Signing value 42
	// Signing value 42
	// Signing value 42
	// Success true
}

type exampleMessage struct {
	Value string
}

func (msg exampleMessage) Serialize(ctx serde.Context) ([]byte, error) {
	return ctx.Marshal(msg)
}

type exampleReactor struct{}

func (exampleReactor) Invoke(from mino.Address, msg serde.Message) ([]byte, error) {
	example, ok := msg.(exampleMessage)
	if !ok {
		return nil, errors.New("unsupported message")
	}

	fmt.Println("Signing value", example.Value)

	return []byte(example.Value), nil
}

func (exampleReactor) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	var msg exampleMessage
	err := ctx.Unmarshal(data, &msg)

	return msg, err
}
