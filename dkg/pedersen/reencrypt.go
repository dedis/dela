package pedersen

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/net/context"
	"golang.org/x/xerrors"
)

type reencryptStatus struct {
	K    kyber.Point // K is the random part of the encrypted secret
	pubk kyber.Point // The client's public key

	nbnodes    int // How many nodes participate in the distributed operations
	nbfailures int // How many failures occurred so far
	threshold  int // How many replies are needed to re-create the secret

	replies []types.ReencryptReply // replies received
	Uis     []*share.PubShare      // re-encrypted shares
}

// Reencrypt implements dkg.Actor.
func (a *Actor) Reencrypt(K kyber.Point, pubk kyber.Point) (XhatEnc kyber.Point, err error) {
	if !a.startRes.Done() {
		return nil, xerrors.Errorf(initDkgFirst)
	}

	ctx, cancel := context.WithTimeout(context.Background(), decryptTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, tracing.ProtocolKey, protocolNameReencrypt)

	players := mino.NewAddresses(a.startRes.getParticipants()...)

	sender, receiver, err := a.rpc.Stream(ctx, players)
	if err != nil {
		return nil, xerrors.Errorf(failedStreamCreation, err)
	}

	iterator := players.AddressIterator()
	addrs := make([]mino.Address, 0, players.Len())

	for iterator.HasNext() {
		addrs = append(addrs, iterator.GetNext())
	}

	txMsg := types.NewReencryptRequest(K, pubk)

	err = <-sender.Send(txMsg, addrs...)
	if err != nil {
		return nil, xerrors.Errorf("failed to send reencrypt request: %v", err)
	}

	status := &reencryptStatus{
		K:    K,
		pubk: pubk,
	}
	status.nbnodes = len(addrs)
	status.threshold = a.startRes.getThreshold()

	for i := 0; i < status.nbnodes; i++ {
		src, rxMsg, err := receiver.Recv(ctx)
		if err != nil {
			return nil, xerrors.Errorf(unexpectedStreamStop, err)
		}

		dela.Logger.Debug().Msgf("Received a decryption reply from %v", src)

		reply, ok := rxMsg.(types.ReencryptReply)
		if !ok {
			return nil, xerrors.Errorf(unexpectedReply, reply, rxMsg)
		}

		err = status.processReencryptReply(&reply)
		if err == nil {
			dela.Logger.Debug().Msgf("Reencryption Uis: %v", status.Uis)

			XhatEnc, err := share.RecoverCommit(suites.MustFind("Ed25519"), status.Uis, status.threshold, status.nbnodes)
			if err != nil {
				return nil, xerrors.Errorf("Reencryption failed: %v", err)
			}

			// we have successfully reencrypted
			return XhatEnc, nil
		}
	}

	return nil, xerrors.Errorf("Reencryption failed: %v", err)
}

func (s *reencryptStatus) processReencryptReply(reply *types.ReencryptReply) (err error) {
	if reply.Ui == nil {
		err = xerrors.Errorf("Received empty reply")
		dela.Logger.Warn().Msg("Empty reply received")
		s.nbfailures++
		if s.nbfailures > s.nbnodes-s.threshold {
			err = xerrors.Errorf("couldn't get enough shares")
			dela.Logger.Warn().Msg(err.Error())
		}
		return err
	}

	s.replies = append(s.replies, *reply)

	if len(s.replies) >= s.threshold {
		s.Uis = make([]*share.PubShare, 0, s.nbnodes)

		for _, r := range s.replies {

			/*
				// Verify proofs
				ufi := suite.Point().Mul(r.Fi, suite.Point().Add(s.U, s.pubk))
				uiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), r.Ui.V)
				uiHat := suite.Point().Add(ufi, uiei)

				gfi := suite.Point().Mul(r.Fi, nil)
				gxi := s.poly.Eval(r.Ui.I).V
				hiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), gxi)
				hiHat := suite.Point().Add(gfi, hiei)
				hash := sha256.New()
				r.Ui.V.MarshalTo(hash)
				uiHat.MarshalTo(hash)
				hiHat.MarshalTo(hash)
				e := suite.Scalar().SetBytes(hash.Sum(nil))
				if e.Equal(r.Ei) {

			*/
			s.Uis = append(s.Uis, r.Ui)
			/*
				}
				else
				{
					dela.Logger.Warn().Msgf("Received invalid share from node: %v", r.Ui.I)
					s.nbfailures++
				}
			*/
		}
		dela.Logger.Info().Msg("Reencryption completed")
		return nil
	}

	// If we are leaving by here it means that we do not have
	// enough replies yet. We must eventually trigger a finish()
	// somehow. It will either happen because we get another
	// reply, and now we have enough, or because we get enough
	// failures and know to give up, or because o.timeout triggers
	// and calls finish(false) in its callback function.

	err = xerrors.Errorf("not enough replies")
	dela.Logger.Debug().Msg(err.Error())
	return err
}

// min is a helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
