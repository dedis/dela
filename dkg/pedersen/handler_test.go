package pedersen

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
)

func TestHandler_Stream(t *testing.T) {
	h := Handler{
		startRes:  &state{},
		deals:     make(chan dealFrom, 10),
		responses: make(chan responseFrom, 10),
	}
	receiver := fake.NewBadReceiver()
	err := h.Stream(fake.Sender{}, receiver)
	require.EqualError(t, err, fake.Err("failed to receive"))

	receiver = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.Deal{}),
		fake.NewRecvMsg(fake.NewAddress(0), types.DecryptRequest{}),
	)
	err = h.Stream(fake.Sender{}, receiver)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")

	h.startRes.distrKey = suite.Point()
	h.startRes.participants = []mino.Address{fake.NewAddress(0)}
	h.privShare = &share.PriShare{I: 0, V: suite.Scalar()}
	receiver = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.DecryptRequest{C: suite.Point()}),
	)
	err = h.Stream(fake.NewBadSender(), receiver)
	require.EqualError(t, err, fake.Err("got an error while sending the decrypt reply"))

	receiver = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), fake.Message{}),
	)
	err = h.Stream(fake.Sender{}, receiver)
	require.EqualError(t, err, "expected Start message, decrypt request or Deal as first message, got: fake.Message")
}

func TestHandler_Start(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)

	h := Handler{
		startRes:  &state{},
		privKey:   privKey,
		deals:     make(chan dealFrom, 10),
		responses: make(chan responseFrom, 10),
	}
	start := types.NewStart(
		0,
		[]mino.Address{fake.NewAddress(0)},
		[]kyber.Point{},
	)
	from := fake.NewAddress(0)
	err := h.start(context.Background(), start, from, fake.Sender{})
	require.EqualError(t, err, "there should be as many participants as pubKey: 1 != 0")

	start = types.NewStart(
		2,
		[]mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		[]kyber.Point{pubKey, suite.Point()},
	)

	h.deals <- dealFrom{
		&types.Deal{},
		fake.NewAddress(0),
	}
	err = h.start(context.Background(), start, from, fake.Sender{})
	require.EqualError(t, err, "failed to receive deals: "+
		"failed to handle received deal: "+
		"failed to process deal from %!s(<nil>): "+
		"schnorr: signature of invalid length 0 instead of 64")
}

func TestHandler_CertifyCanTimeOut(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)

	dkg, err := pedersen.NewDistKeyGenerator(suite, privKey, []kyber.Point{pubKey, suite.Point()}, 2)
	require.NoError(t, err)

	h := Handler{
		startRes: &state{},
		dkg:      dkg,
	}

	ctx, _ := context.WithTimeout(context.Background(), 0*time.Second)
	err = h.certify(ctx)
	require.EqualError(t, err, "timed out while receiving responses")
}

func TestHandler_CertifyTimesOutWithoutValidResponses(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)

	dkg, err := pedersen.NewDistKeyGenerator(suite, privKey, []kyber.Point{pubKey, suite.Point()}, 2)
	require.NoError(t, err)

	h := Handler{
		startRes:  &state{},
		dkg:       dkg,
		deals:     make(chan dealFrom, 10),
		responses: make(chan responseFrom, 10),
	}

	resp := responseFrom{
		&types.Response{},
		fake.NewAddress(0),
	}
	h.responses <- resp
	ctx, _ := context.WithTimeout(context.Background(), 1*time.Second)
	err = h.certify(ctx)
	require.EqualError(t, err, "timed out while receiving responses")
}

// TODO: TestHandler_CertifySucceedWithValidResponses

func TestHandler_HandleDeal(t *testing.T) {
	privKey1 := suite.Scalar().Pick(suite.RandomStream())
	pubKey1 := suite.Point().Mul(privKey1, nil)
	privKey2 := suite.Scalar().Pick(suite.RandomStream())
	pubKey2 := suite.Point().Mul(privKey2, nil)

	dkg1, err := pedersen.NewDistKeyGenerator(suite, privKey1, []kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)

	dkg2, err := pedersen.NewDistKeyGenerator(suite, privKey2, []kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)

	deals, err := dkg2.Deals()
	require.Len(t, deals, 1)
	require.NoError(t, err)

	var deal *pedersen.Deal
	for _, d := range deals {
		deal = d
	}

	dealMsg := types.NewDeal(
		deal.Index,
		deal.Signature,
		types.NewEncryptedDeal(
			deal.Deal.DHKey,
			deal.Deal.Signature,
			deal.Deal.Nonce,
			deal.Deal.Cipher,
		),
	)

	h := Handler{
		dkg: dkg1,
	}
	err = h.handleDeal(&dealMsg, nil, []mino.Address{fake.NewAddress(0)}, fake.NewBadSender())
	require.EqualError(t, err, fake.Err("failed to send response to 'fake.Address[0]'"))
}

// -----------------------------------------------------------------------------
// Utility functions
/*
func getCertified(t *testing.T) *pedersen.DistKeyGenerator {
	privKey1 := suite.Scalar().Pick(suite.RandomStream())
	pubKey1 := suite.Point().Mul(privKey1, nil)

	privKey2 := suite.Scalar().Pick(suite.RandomStream())
	pubKey2 := suite.Point().Mul(privKey2, nil)

	dkg1, err := pedersen.NewDistKeyGenerator(suite, privKey1,
		[]kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)
	dkg2, err := pedersen.NewDistKeyGenerator(suite, privKey2,
		[]kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)

	deals1, err := dkg1.Deals()
	require.NoError(t, err)
	require.Len(t, deals1, 1)
	deals2, err := dkg2.Deals()
	require.Len(t, deals2, 1)
	require.NoError(t, err)

	var resp1 *pedersen.Response
	var resp2 *pedersen.Response

	for _, deal := range deals2 {
		resp1, err = dkg1.ProcessDeal(deal)
		require.NoError(t, err)
	}
	for _, deal := range deals1 {
		resp2, err = dkg2.ProcessDeal(deal)
		require.NoError(t, err)
	}

	_, err = dkg1.ProcessResponse(resp2)
	require.NoError(t, err)
	_, err = dkg2.ProcessResponse(resp1)
	require.NoError(t, err)

	require.True(t, dkg1.Certified())
	require.True(t, dkg2.Certified())

	return dkg1
}
*/
