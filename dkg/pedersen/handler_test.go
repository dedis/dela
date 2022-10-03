package pedersen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
)

func TestHandler_Stream(t *testing.T) {
	h := Handler{
		startRes: &state{},
	}

	receiver := fake.NewBadReceiver()
	err := h.Stream(fake.Sender{}, receiver)
	require.EqualError(t, err, fake.Err("failed to receive"))

	receiver = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.Deal{}),
		fake.NewRecvMsg(fake.NewAddress(0), types.DecryptRequest{}),
	)

	err = h.Stream(fake.Sender{}, receiver)
	require.EqualError(t, err, "bad state: unexpected state: Initial != one of [Certified]")

	h.startRes.distrKey = suite.Point()
	h.startRes.participants = []mino.Address{fake.NewAddress(0)}
	h.privShare = &share.PriShare{I: 0, V: suite.Scalar()}
	receiver = fake.NewReceiver(
		fake.NewRecvMsg(fake.NewAddress(0), types.DecryptRequest{C: suite.Point()}),
	)
	h.startRes.dkgState = certified

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
		startRes: &state{},
		privKey:  privKey,
	}
	start := types.NewStart(0, []mino.Address{fake.NewAddress(0)}, []kyber.Point{})
	from := fake.NewAddress(0)

	err := h.start(context.Background(), start, cryChan[types.Deal]{}, cryChan[types.Response]{}, from, fake.Sender{})
	require.EqualError(t, err, "there should be as many participants as pubKey: 1 != 0")

	h.startRes.dkgState = initial

	start = types.NewStart(2, []mino.Address{fake.NewAddress(0), fake.NewAddress(1)}, []kyber.Point{pubKey, suite.Point()})

	err = h.start(context.Background(), start, cryChan[types.Deal]{}, cryChan[types.Response]{}, from, fake.Sender{})
	require.NoError(t, err)
}

func TestHandler_Deal_Ctx_Fail(t *testing.T) {
	privKey1 := suite.Scalar().Pick(suite.RandomStream())
	pubKey1 := suite.Point().Mul(privKey1, nil)
	privKey2 := suite.Scalar().Pick(suite.RandomStream())
	pubKey2 := suite.Point().Mul(privKey2, nil)

	dkg, err := pedersen.NewDistKeyGenerator(suite, privKey1, []kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)

	h := Handler{
		dkg: dkg,
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = h.deal(ctx, noSender{})
	require.EqualError(t, err, "context done: context canceled")
}

func TestHandler_Respond_Ctx_Fail(t *testing.T) {
	h := Handler{startRes: &state{
		participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := h.respond(ctx, newCryChan[types.Deal](1), nil)
	require.EqualError(t, err, "context done: context canceled")
}

func TestHandler_CertifyCanSucceed(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)

	dkg, err := pedersen.NewDistKeyGenerator(suite, privKey, []kyber.Point{pubKey, suite.Point()}, 2)
	require.NoError(t, err)

	h := Handler{
		startRes: &state{dkgState: sharing},
		dkg:      dkg,
	}

	responses := newCryChan[types.Response](1)

	dkg, resp := getCertified(t)

	msg := types.NewResponse(
		resp.Index,
		types.NewDealerResponse(
			resp.Response.Index,
			resp.Response.Status,
			resp.Response.SessionID,
			resp.Response.Signature,
		),
	)

	responses.push(msg)

	h.dkg = dkg
	err = h.certify(context.Background(), responses, 1)
	require.NoError(t, err)
}

func TestHandler_Certify_Ctx_Fail(t *testing.T) {
	h := Handler{
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := h.certify(ctx, newCryChan[types.Response](1), 1)
	require.EqualError(t, err, "context done: context canceled")
}

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
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}
	err = h.handleDeal(context.Background(), dealMsg, fake.NewBadSender(), h.startRes.getParticipants())
	require.EqualError(t, err, fake.Err("failed to send response to 'fake.Address[0]'"))
}

func TestHandler_HandleDeal_Ctx_Fail(t *testing.T) {
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
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	err = h.handleDeal(ctx, dealMsg, noSender{}, h.startRes.getParticipants())
	require.EqualError(t, err, "context done: context canceled")
}

// -----------------------------------------------------------------------------
// Utility functions

func getCertified(t *testing.T) (*pedersen.DistKeyGenerator, *pedersen.Response) {
	privKey1 := suite.Scalar().Pick(suite.RandomStream())
	pubKey1 := suite.Point().Mul(privKey1, nil)

	privKey2 := suite.Scalar().Pick(suite.RandomStream())
	pubKey2 := suite.Point().Mul(privKey2, nil)

	dkg1, err := pedersen.NewDistKeyGenerator(suite, privKey1, []kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)
	dkg2, err := pedersen.NewDistKeyGenerator(suite, privKey2, []kyber.Point{pubKey1, pubKey2}, 2)
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

	_, err = dkg2.ProcessResponse(resp1)
	require.NoError(t, err)

	return dkg1, resp2
}

// implements a sender that never returns
//
// - implements mino.Sender
type noSender struct {
	mino.Sender
}

// Send implements mino.Sender.
func (s noSender) Send(serde.Message, ...mino.Address) <-chan error {
	errs := make(chan error, 1)
	return errs
}
