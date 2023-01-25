package pedersen

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/dedis/debugtools/channel"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
)

func TestDKGInstance_IsRunning(t *testing.T) {
	s := instance{
		running: false,
	}

	require.False(t, s.isRunning())

	s.running = true
	require.True(t, s.isRunning())
}

func TestDKGInstance_HandleStartFail(t *testing.T) {
	out := &bytes.Buffer{}
	log := zerolog.New(out)

	s := instance{
		startRes: &state{dkgState: certified},
		log:      log,
	}

	err := s.handleMessage(context.TODO(), types.Start{}, fake.NewAddress(0), nil)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 100)

	require.Regexp(t, "failed to start", out.String())
}

func TestDKGInstance_HandleStartResharingFail(t *testing.T) {
	out := &bytes.Buffer{}
	log := zerolog.New(out)

	s := instance{
		startRes: &state{dkgState: sharing},
		log:      log,
	}

	err := s.handleMessage(context.TODO(), types.StartResharing{}, fake.NewAddress(0), nil)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 100)

	require.Regexp(t, "failed to handle resharing", out.String())
}

func TestDKGInstance_HandleDealFail(t *testing.T) {
	s := instance{
		startRes: &state{dkgState: 0xaa},
	}

	err := s.handleMessage(context.TODO(), types.Deal{}, fake.NewAddress(0), nil)
	require.EqualError(t, err, "bad state: unexpected state: "+
		"UNKNOWN != one of [Initial Sharing Certified Resharing]")
}

func TestDKGInstance_HandleReshareFail(t *testing.T) {
	s := instance{
		startRes: &state{dkgState: 0xaa},
	}

	err := s.handleMessage(context.TODO(), types.Reshare{}, fake.NewAddress(0), nil)
	require.EqualError(t, err, "bad state: unexpected state: "+
		"UNKNOWN != one of [Initial Certified Resharing]")
}

func TestDKGInstance_HandleResponseFail(t *testing.T) {
	s := instance{
		startRes: &state{dkgState: 0xaa},
	}

	err := s.handleMessage(context.TODO(), types.Response{}, fake.NewAddress(0), nil)
	require.EqualError(t, err, "bad state: unexpected state: "+
		"UNKNOWN != one of [Initial Sharing Certified Resharing]")
}

func TestDKGInstance_HandleDecryptRequestFail(t *testing.T) {
	s := instance{
		startRes: &state{dkgState: 0xaa},
	}

	err := s.handleMessage(context.TODO(), types.DecryptRequest{},
		fake.NewAddress(0), nil)

	require.EqualError(t, err, "bad state: unexpected state: UNKNOWN != one of [Certified]")
}

func TestDKGInstance_HandleVerifiableDecryptRequestFail(t *testing.T) {
	s := instance{
		startRes: &state{dkgState: 0xaa},
	}

	err := s.handleMessage(context.TODO(), types.VerifiableDecryptRequest{},
		fake.NewAddress(0), nil)

	require.EqualError(t, err, "bad state: unexpected state: UNKNOWN != one of [Certified]")
}

func TestDKGInstance_HandleUnknown(t *testing.T) {
	s := instance{
		startRes: &state{dkgState: 0xaa},
	}

	err := s.handleMessage(context.TODO(), fake.Message{}, fake.NewAddress(0), nil)

	require.EqualError(t, err, "expected Start message, decrypt request or "+
		"Deal as first message, got: fake.Message")
}

func TestDKGInstance_StartFailNewDKG(t *testing.T) {
	s := instance{
		startRes: &state{},
	}

	err := s.start(context.TODO(), types.Start{}, channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, nil, nil)

	require.EqualError(t, err, "failed to create new DKG: dkg: can't run with empty node list")
}

func TestDKGInstance_Start(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)

	s := instance{
		startRes: &state{},
		privKey:  privKey,
		log:      dela.Logger,
	}
	start := types.NewStart(0, []mino.Address{fake.NewAddress(0)}, []kyber.Point{})
	from := fake.NewAddress(0)

	err := s.start(context.Background(), start, channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, from, fake.Sender{})

	require.EqualError(t, err, "there should be as many participants as pubKey: 1 != 0")

	s.startRes.dkgState = initial

	start = types.NewStart(2, []mino.Address{fake.NewAddress(0),
		fake.NewAddress(1)}, []kyber.Point{pubKey, suite.Point()})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = s.start(ctx, start, channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, from, fake.Sender{})
	require.NoError(t, err)
}

func TestDKGInstance_doDKG_DealFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{dealsErr: fake.GetError()},
	}

	err := s.doDKG(context.TODO(), channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, nil, nil)

	require.EqualError(t, err, fake.Err("failed to deal: failed to compute the deals"))
}

func TestDKGInstance_doDKG_RespondFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			respoErr: fake.GetError(),
		},
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.doDKG(ctx, channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, nil, nil)

	require.EqualError(t, err, "failed to respond: context done: Could not "+
		"receive data from channel.")
}

func TestDKGInstance_doDKG_CertifyFail(t *testing.T) {
	s := instance{
		dkg:      fakeDKGService{},
		startRes: &state{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.doDKG(ctx, channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, nil, nil)

	require.EqualError(t, err, "failed to certify: context done: Could not "+
		"receive data from channel.")
}

func TestDKGInstance_doDKG_SwitchFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			certified: true,
		},
		startRes: &state{
			dkgState:     initial,
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}

	err := s.doDKG(context.Background(), channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, nil, nil)

	require.EqualError(t, err, "failed to switch state: certified state must "+
		"switch from sharing or resharing: Initial")
}

func TestDKGInstance_doDKG_FinalizeFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			certified: true,
			shareErr:  fake.GetError(),
		},
		startRes: &state{
			dkgState:     sharing,
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}

	err := s.doDKG(context.Background(), channel.Timed[types.Deal]{},
		channel.Timed[types.Response]{}, nil, nil)

	require.EqualError(t, err, fake.Err("failed to finalize: failed to get distr key"))
}

func TestDKGInstance_deal_SendFail(t *testing.T) {
	deals := map[int]*pedersen.Deal{0: {Deal: &vss.EncryptedDeal{}}}

	out := &bytes.Buffer{}
	log := zerolog.New(out)

	s := instance{
		dkg: fakeDKGService{
			deals: deals,
		},
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0)},
		},
		log: log,
	}

	err := s.deal(context.Background(), fake.NewBadSender())
	require.NoError(t, err)

	require.Regexp(t, "failed to send deal", out.String())
}

func TestDKGInstance_deal_ctxFail(t *testing.T) {
	privKey1 := suite.Scalar().Pick(suite.RandomStream())
	pubKey1 := suite.Point().Mul(privKey1, nil)
	privKey2 := suite.Scalar().Pick(suite.RandomStream())
	pubKey2 := suite.Point().Mul(privKey2, nil)

	dkg, err := pedersen.NewDistKeyGenerator(suite, privKey1, []kyber.Point{pubKey1, pubKey2}, 2)
	require.NoError(t, err)

	s := instance{
		dkg: dkg,
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = s.deal(ctx, blockingSender{})
	require.EqualError(t, err, "context done: context canceled")
}

func TestDKGInstance_respond_handleFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			processErr: fake.GetError(),
		},
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		},
	}

	deals := channel.WithExpiration[types.Deal](1)
	deals.Send(types.Deal{})

	err := s.respond(context.Background(), deals, fake.NewBadSender())
	require.EqualError(t, err, fake.Err("failed to handle received deal: failed to process deal"))
}

func TestDKGInstance_respond_ctxFail(t *testing.T) {
	s := instance{startRes: &state{
		participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.respond(ctx, channel.WithExpiration[types.Deal](1), nil)
	require.EqualError(t, err, "context done: Could not receive data from channel.")
}

func TestDKGInstance_certifyProcessFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			respoErr: fake.GetError(),
		},
	}

	resps := channel.WithExpiration[types.Response](1)
	resps.Send(types.NewResponse(0, types.NewDealerResponse(0, false, nil, nil)))

	err := s.certify(context.Background(), resps, 1)
	require.EqualError(t, err, fake.Err("failed to process response"))
}

func TestDKGInstance_certifyCertifyFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			certified: false,
		},
	}

	resps := channel.WithExpiration[types.Response](1)
	resps.Send(types.NewResponse(0, types.NewDealerResponse(0, false, nil, nil)))

	err := s.certify(context.Background(), resps, 1)
	require.EqualError(t, err, "node is not certified")
}

func TestDKGInstance_certifyOK(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)

	dkg, err := pedersen.NewDistKeyGenerator(suite, privKey,
		[]kyber.Point{pubKey, suite.Point()}, 2)
	require.NoError(t, err)

	s := instance{
		startRes: &state{dkgState: sharing},
		dkg:      dkg,
	}

	responses := channel.WithExpiration[types.Response](1)

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

	responses.Send(msg)

	s.dkg = dkg
	err = s.certify(context.Background(), responses, 1)
	require.NoError(t, err)
}

func TestDKGInstance_certify_ctxFail(t *testing.T) {
	s := instance{
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0), fake.NewAddress(1)},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.certify(ctx, channel.WithExpiration[types.Response](1), 1)
	require.EqualError(t, err, "context done: Could not receive data from channel.")
}

func TestDKGInstance_finalize_sendFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			distKeyShare: &pedersen.DistKeyShare{
				Commits: []kyber.Point{suite.Point()},
			},
		},
		startRes: &state{},
	}

	err := s.finalize(context.Background(), nil, fake.NewBadSender())
	require.EqualError(t, err, fake.Err("got an error while sending pub key"))
}

func TestDKGInstance_finalize_ctxFail(t *testing.T) {
	out := &bytes.Buffer{}
	log := zerolog.New(out)

	s := instance{
		dkg: fakeDKGService{
			distKeyShare: &pedersen.DistKeyShare{
				Commits: []kyber.Point{suite.Point()},
			},
		},
		startRes: &state{},
		log:      log,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.finalize(ctx, nil, blockingSender{})
	require.NoError(t, err)

	require.Regexp(t, "context done", out.String())
}

func TestDKGInstance_finalizeReshare_sistKeyFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			shareErr: fake.GetError(),
		},
		startRes: &state{},
	}

	err := s.finalizeReshare(context.TODO(), commonNode, nil, nil)
	require.EqualError(t, err, fake.Err("failed to get distr key"))
}

func TestDKGInstance_finalizeReshare_sendFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			distKeyShare: &pedersen.DistKeyShare{Commits: []kyber.Point{suite.Point()}},
		},
		startRes: &state{},
	}

	err := s.finalizeReshare(context.Background(), commonNode, fake.NewBadSender(), nil)
	require.EqualError(t, err, fake.Err("got an error while sending pub key"))
}

func TestDKGInstance_finalizeReshare_ctxFail(t *testing.T) {
	out := &bytes.Buffer{}
	log := zerolog.New(out)

	s := instance{
		dkg: fakeDKGService{
			distKeyShare: &pedersen.DistKeyShare{Commits: []kyber.Point{suite.Point()}},
		},
		startRes: &state{},
		log:      log,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.finalizeReshare(ctx, commonNode, blockingSender{}, nil)
	require.NoError(t, err)

	require.Regexp(t, "context done", out.String())
}

func TestDKGInstance_reshare_lenFail(t *testing.T) {
	s := instance{
		startRes: &state{},
	}

	msg := types.NewStartResharing(0, 0, make([]mino.Address, 1), nil, nil, nil)

	err := s.reshare(context.TODO(), nil, nil, msg,
		channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})

	require.EqualError(t, err, "there should be as many participants as pubKey: 1 != 0")
}

func TestDKGInstance_reshare_doDKGFail(t *testing.T) {
	s := instance{
		startRes: &state{
			dkgState: resharing,
		},
		dkg: fakeDKGService{
			certified: false,
		},
	}

	msg := types.NewStartResharing(0, 0, nil, nil, nil, nil)

	err := s.reshare(context.TODO(), nil, nil, msg,
		channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})

	require.EqualError(t, err, "failed to switch state: resharing state must "+
		"switch from initial or certified: Resharing")
}

func TestDKGInstance_doReshare_oldNode_getDistKeyFail(t *testing.T) {
	s := instance{
		startRes: &state{
			distrKey: suite.Point(),
		},
		dkg: fakeDKGService{
			shareErr: fake.GetError(),
		},
	}

	err := s.doReshare(context.TODO(), types.StartResharing{}, nil, nil,
		channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})

	require.EqualError(t, err, fake.Err("old node failed to create"))
}

func TestDKGInstance_doReshare_oldNode_NewDistKeyFail(t *testing.T) {
	s := instance{
		startRes: &state{
			distrKey: suite.Point(),
		},
		dkg: fakeDKGService{},
	}

	err := s.doReshare(context.TODO(), types.StartResharing{}, nil, nil,
		channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})

	require.EqualError(t, err, "old node failed to compute the new dkg: dkg: "+
		"can't run with empty node list")
}

func TestDKGInstance_doReshare_oldNode_sendDealFail(t *testing.T) {
	// We need to have an actual correct DKG for resharing.
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

	_, err = dkg1.ProcessResponse(resp2)
	require.NoError(t, err)

	s := instance{
		startRes: &state{
			distrKey:     suite.Point(),
			participants: []mino.Address{},
			pubkeys:      []kyber.Point{pubKey1, pubKey2},
			threshold:    2,
		},
		dkg:     dkg1,
		me:      fake.NewAddress(0),
		privKey: privKey1,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{fake.NewAddress(0),
		fake.NewAddress(1)}, []mino.Address{fake.NewAddress(0)},
		[]kyber.Point{pubKey1, pubKey2}, nil)

	err = s.doReshare(context.Background(), msg, fake.NewAddress(0),
		fake.NewBadSender(), channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})

	require.EqualError(t, err, fake.Err("old node failed to send deals: "+
		"failed to send resharing deal"))
}

func TestDKGInstance_doReshare_commonNode_getDistKeyFail(t *testing.T) {
	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			distrKey:     suite.Point(),
			participants: []mino.Address{me},
		},
		dkg: fakeDKGService{
			shareErr: fake.GetError(),
		},
		me: me,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{me}, nil, nil)

	err := s.doReshare(context.TODO(), msg, nil, nil,
		channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})
	require.EqualError(t, err, fake.Err("common node failed to create"))
}

func TestDKGInstance_doReshare_commonNode_NewDistKeyFail(t *testing.T) {
	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			distrKey:     suite.Point(),
			participants: []mino.Address{me},
		},
		dkg: fakeDKGService{},
		me:  me,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{me}, nil, nil)

	err := s.doReshare(context.TODO(), msg, nil, nil,
		channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})
	require.EqualError(t, err, "common node failed to compute the new dkg: "+
		"dkg: can't run with empty node list")
}

func TestDKGInstance_doReshare_commonNode_sendDealFail(t *testing.T) {
	// We need to have an actual correct DKG for resharing.
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

	_, err = dkg1.ProcessResponse(resp2)
	require.NoError(t, err)

	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			distrKey:     suite.Point(),
			participants: []mino.Address{me},
			pubkeys:      []kyber.Point{pubKey1, pubKey2},
			threshold:    2,
		},
		dkg:     dkg1,
		me:      me,
		privKey: privKey1,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{fake.NewAddress(0)}, []kyber.Point{pubKey1, pubKey2}, nil)

	err = s.doReshare(context.Background(), msg, fake.NewAddress(0),
		fake.NewBadSender(), channel.Timed[types.Reshare]{}, channel.Timed[types.Response]{})

	require.EqualError(t, err, fake.Err("common node failed to send deals: "+
		"failed to send resharing deal"))
}

func TestDKGInstance_doReshare_commonNode_receiveDealFail(t *testing.T) {
	// We need to have an actual correct DKG for resharing.
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

	_, err = dkg1.ProcessResponse(resp2)
	require.NoError(t, err)

	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			distrKey:     suite.Point(),
			participants: []mino.Address{me},
			pubkeys:      []kyber.Point{pubKey1, pubKey2},
			threshold:    2,
		},
		dkg:     dkg1,
		me:      me,
		privKey: privKey1,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{fake.NewAddress(0)}, []kyber.Point{pubKey1, pubKey2}, nil)

	reshares := channel.WithExpiration[types.Reshare](2)
	reshares.Send(types.Reshare{})
	reshares.Send(types.Reshare{})

	err = s.doReshare(context.Background(), msg, fake.NewAddress(0),
		fake.Sender{}, reshares, channel.Timed[types.Response]{})

	require.Regexp(t, "^common node failed to receive deals", err.Error())
}

func TestDKGInstance_doReshare_newNode_ctxFail(t *testing.T) {
	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(2)},
		},
		dkg: fakeDKGService{},
		me:  me,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{fake.NewAddress(2)}, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.doReshare(ctx, msg, nil, nil, channel.Timed[types.Reshare]{},
		channel.Timed[types.Response]{})

	require.EqualError(t, err, "new node failed to receive deals: "+
		"context done: Could not receive data from channel.")
}

func TestDKGInstance_doReshare_certifyFail(t *testing.T) {
	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			dkgState:     initial,
			participants: []mino.Address{fake.NewAddress(2)},
		},
		dkg: fakeDKGService{
			certified: true,
		},
		me: me,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{}, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.doReshare(ctx, msg, nil, nil, channel.Timed[types.Reshare]{},
		channel.Timed[types.Response]{})
	require.EqualError(t, err, "failed to certify: context done: "+
		"Could not receive data from channel.")
}

func TestDKGInstance_doReshare_switchStateFail(t *testing.T) {
	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			dkgState:     initial,
			participants: []mino.Address{fake.NewAddress(2)},
		},
		dkg: fakeDKGService{
			certified: true,
		},
		me: me,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{}, nil, nil)

	responses := channel.WithExpiration[types.Response](2)
	responses.Send(types.Response{})
	responses.Send(types.Response{})

	err := s.doReshare(context.Background(), msg, nil, nil,
		channel.Timed[types.Reshare]{}, responses)

	require.EqualError(t, err, "failed to switch state: "+
		"certified state must switch from sharing or resharing: Initial")
}

func TestDKGInstance_doReshare_finalizeFail(t *testing.T) {
	me := fake.NewAddress(0)

	s := instance{
		startRes: &state{
			dkgState:     sharing,
			participants: []mino.Address{fake.NewAddress(2)},
		},
		dkg: fakeDKGService{
			certified:    true,
			distKeyShare: &pedersen.DistKeyShare{Commits: []kyber.Point{suite.Point()}},
		},
		me: me,
	}

	msg := types.NewStartResharing(2, 2, []mino.Address{me, fake.NewAddress(1)},
		[]mino.Address{}, nil, nil)

	responses := channel.WithExpiration[types.Response](2)
	responses.Send(types.Response{})
	responses.Send(types.Response{})

	err := s.doReshare(context.Background(), msg, nil, fake.NewBadSender(),
		channel.Timed[types.Reshare]{}, responses)

	require.EqualError(t, err, fake.Err("failed to announce dkg public key: "+
		"got an error while sending pub key"))
}

func TestDKGInstance_sendDealsResharing_dealsFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			dealsErr: fake.GetError(),
		},
	}

	err := s.sendDealsResharing(context.TODO(), nil, nil, nil)
	require.EqualError(t, err, fake.Err("failed to compute the deals"))
}

func TestDKGInstance_sendDealsResharing_ctxFail(t *testing.T) {
	s := instance{
		dkg: fakeDKGService{
			deals: map[int]*pedersen.Deal{0: {Deal: &vss.EncryptedDeal{}}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.sendDealsResharing(ctx, blockingSender{}, make([]mino.Address, 1), nil)
	require.EqualError(t, err, "context done: context canceled")
}

func TestDKGInstance_handleDecrypt_notStarted(t *testing.T) {
	s := instance{
		startRes: &state{},
	}

	err := s.handleDecrypt(nil, types.DecryptRequest{}, nil)
	require.EqualError(t, err, "you must first initialize DKG. Did you call setup() first?")
}

func TestDKGInstance_handleDecrypt_sendFail(t *testing.T) {
	s := instance{
		startRes: &state{
			dkgState: certified,
		},
		privShare: &share.PriShare{V: suite.Scalar()},
	}

	req := types.DecryptRequest{K: suite.Point(), C: suite.Point()}

	err := s.handleDecrypt(fake.NewBadSender(), req, nil)
	require.EqualError(t, err, fake.Err("got an error while sending the decrypt reply"))
}

func TestDKGInstance_handleDeal_responseFail(t *testing.T) {
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

	s := instance{
		dkg: dkg1,
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}

	err = s.handleDeal(context.Background(), dealMsg, fake.NewBadSender(),
		s.startRes.getParticipants())

	require.EqualError(t, err, fake.Err("failed to send response to 'fake.Address[0]'"))
}

func TestDKGInstance_handleDeal_ctxFail(t *testing.T) {
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

	s := instance{
		dkg: dkg1,
		startRes: &state{
			participants: []mino.Address{fake.NewAddress(0)},
		},
	}

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	err = s.handleDeal(ctx, dealMsg, blockingSender{}, s.startRes.getParticipants())
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

type fakeDKGService struct {
	dkgService

	dealsErr   error
	respoErr   error
	shareErr   error
	processErr error

	certified bool

	deals        map[int]*pedersen.Deal
	distKeyShare *pedersen.DistKeyShare
}

func (s fakeDKGService) Deals() (map[int]*pedersen.Deal, error) {
	return s.deals, s.dealsErr
}

func (s fakeDKGService) ProcessResponse(resp *pedersen.Response) (*pedersen.Justification, error) {
	return nil, s.respoErr
}

func (s fakeDKGService) Certified() bool {
	return s.certified
}

func (s fakeDKGService) DistKeyShare() (*pedersen.DistKeyShare, error) {
	return s.distKeyShare, s.shareErr
}

func (s fakeDKGService) ProcessDeal(dd *pedersen.Deal) (*pedersen.Response, error) {
	return nil, s.processErr
}

type blockingSender struct {
}

func (blockingSender) Send(msg serde.Message, addrs ...mino.Address) <-chan error {
	return make(<-chan error)
}
