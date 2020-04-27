package pedersen

import (
	"context"
	"sync"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	pedersen "go.dedis.ch/kyber/v3/share/dkg/pedersen"
	vss "go.dedis.ch/kyber/v3/share/vss/pedersen"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

// Suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

// Handler represents the RPC executed on the nodes
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	af        mino.AddressFactory
	dkg       *pedersen.DistKeyGenerator
	pubKeys   []kyber.Point
	privKey   kyber.Scalar
	me        mino.Address
	suite     suites.Suite
	privShare *share.PriShare
}

// NewHandler ...
func NewHandler(pubKeys []kyber.Point, privKey kyber.Scalar,
	af mino.AddressFactory, me mino.Address, suite suites.Suite) *Handler {

	return &Handler{
		af:      af,
		pubKeys: pubKeys,
		privKey: privKey,
		me:      me,
		suite:   suite,
	}
}

// Stream ...
func (h *Handler) Stream(out mino.Sender, in mino.Receiver) error {

	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("failed to receive: %v", err)
	}

	// We expect a Start message or a decrypt request at first
	switch msg := msg.(type) {
	case *Start:
		err := h.start(msg, from, out, in)
		if err != nil {
			return xerrors.Errorf("failed to start: %v", err)
		}
	case *Decrypt:
		// TODO: check if started before
		K := h.suite.Point()
		err := K.UnmarshalBinary(msg.K)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal K: %v", err)
		}

		C := h.suite.Point()
		err = C.UnmarshalBinary(msg.C)
		if err != nil {
			return xerrors.Errorf("failed tun unmarshal C: %v", err)
		}

		S := suite.Point().Mul(h.privShare.V, K)
		partial := suite.Point().Sub(C, S)

		VBuf, err := partial.MarshalBinary()
		if err != nil {
			return xerrors.Errorf("failed to marshal the partial: %v", err)
		}

		decryptReply := &DecryptReply{
			V: VBuf,
			// TODO: check if using the private index is the same as the public
			// index.
			I: int64(h.privShare.I),
		}

		errs := out.Send(decryptReply, from)
		err, more := <-errs
		if more {
			return xerrors.Errorf("got an error while sending the decrypt "+
				"reply: %v", err)
		}

	default:
		return xerrors.Errorf("expected Start message or decrypt request as "+
			"first message, got %T: %v", msg, msg)
	}
	return nil
}

func (h *Handler) start(start *Start, from mino.Address,
	out mino.Sender, in mino.Receiver) error {

	addrs := make([]mino.Address, len(start.Addresses))
	for i, addrBuf := range start.Addresses {
		addr := h.af.FromText(addrBuf)
		if addr == nil {
			return xerrors.Errorf("failed to unmarsahl address '%s'", addr)
		}
		addrs[i] = addr
	}

	// 1. Create the DKG
	d, err := pedersen.NewDistKeyGenerator(suite, h.privKey, h.pubKeys, int(start.T))
	if err != nil {
		return xerrors.Errorf("failed to create new DKG: %v", err)
	}
	h.dkg = d

	// 2. Send my Deals to the other nodes
	deals, err := d.Deals()
	if err != nil {
		return xerrors.Errorf("failed to compute the deals: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(len(deals))
	for i, deal := range deals {
		dealMsg := &Deal{
			Index: deal.Index,
			EncryptedDeal: &Deal_EncryptedDeal{
				Dhkey:     deal.Deal.DHKey,
				Signature: deal.Deal.Signature,
				Nonce:     deal.Deal.Nonce,
				Cipher:    deal.Deal.Cipher,
			},
			Signature: deal.Signature,
		}

		errs := out.Send(dealMsg, addrs[i])
		go func(errs <-chan error) {
			err, more := <-errs
			if more {
				fabric.Logger.Warn().Msgf("got an error while sending deal: %v", err)
			}
			wg.Done()
		}(errs)
	}
	wg.Wait()

	for !h.dkg.Certified() {

		// 3. Receive the deals or the Responses
		from, msg, err := in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive after sending deals: %v", err)
		}

		switch msg := msg.(type) {
		case *Deal:
			// 4. Process the Deal and Send the response to all the other nodes
			err = h.handleDeal(msg, from, addrs, out)
			if err != nil {
				fabric.Logger.Warn().Msgf("failed to handle received deal "+
					"from %s: %v", from, err)
			}
		case *Response:
			// 5. Processing responses
			fabric.Logger.Trace().Msgf("%s received response from %s", h.me, from)
			response := &pedersen.Response{
				Index: msg.Index,
				Response: &vss.Response{
					SessionID: msg.Response.SessionID,
					Index:     msg.Response.Index,
					Status:    msg.Response.Status,
					Signature: msg.Response.Signature,
				},
			}
			_, err = h.dkg.ProcessResponse(response)
			if err != nil {
				return xerrors.Errorf("failed to process response: %v", err)
			}
		}
	}

	fabric.Logger.Trace().Msgf("%s is certified", h.me)

	// 6. Send back the public DKG key
	distrKey, err := h.dkg.DistKeyShare()
	if err != nil {
		return xerrors.Errorf("failed to get distr key: %v", err)
	}

	distrKeyBuf, err := distrKey.Public().MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal distr pub key: %v", err)
	}

	done := &StartDone{PubKey: distrKeyBuf}
	errs := out.Send(done, from)
	err, more := <-errs
	if more {
		return xerrors.Errorf("got an error while sending pub key: %v", err)
	}

	h.privShare = distrKey.PriShare()

	return nil
}

func (h *Handler) handleDeal(msg *Deal, from mino.Address, addrs []mino.Address,
	out mino.Sender) error {

	fabric.Logger.Trace().Msgf("%s received deal from %s", h.me, from)
	deal := &pedersen.Deal{
		Index: msg.Index,
		Deal: &vss.EncryptedDeal{
			DHKey:     msg.EncryptedDeal.Dhkey,
			Signature: msg.EncryptedDeal.Signature,
			Nonce:     msg.EncryptedDeal.Nonce,
			Cipher:    msg.EncryptedDeal.Cipher,
		},
		Signature: msg.Signature,
	}

	response, err := h.dkg.ProcessDeal(deal)
	if err != nil {
		return xerrors.Errorf("failed to process deal from %s: %v",
			h.me, err)
	}

	respProto := &Response{
		Index: response.Index,
		Response: &Response_Data{
			SessionID: response.Response.SessionID,
			Index:     response.Response.Index,
			Status:    response.Response.Status,
			Signature: response.Response.Signature,
		},
	}

	var wg sync.WaitGroup
	wg.Add(len(addrs))
	for _, addr := range addrs {
		if addr.Equal(h.me) {
			wg.Done()
			continue
		}
		errs := out.Send(respProto, addr)
		go func(errs <-chan error) {
			err, more := <-errs
			if more {
				fabric.Logger.Warn().Msgf("got an error while sending "+
					"response: %v", err)
			}
			wg.Done()
		}(errs)
	}
	wg.Wait()

	return nil
}
