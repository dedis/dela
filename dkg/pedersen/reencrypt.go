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

type onChainSecret struct {
	K    kyber.Point // K is the random part of the encrypted secret
	pubk kyber.Point // The client's public key

	nbnodes    int // How many nodes participate in the distributed operations
	nbfailures int // How many failures occurred so far
	threshold  int // How many replies are needed to re-create the secret

	replies []types.ReencryptReply // replies received
	Uis     []*share.PubShare      // re-encrypted shares
}

// newOCS creates a new on-chain secret structure.
func newOCS(K kyber.Point, pubk kyber.Point) *onChainSecret {
	return &onChainSecret{
		K:    K,
		pubk: pubk,
	}
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

	ocs := newOCS(K, pubk)
	ocs.nbnodes = len(addrs)
	ocs.threshold = a.startRes.getThreshold()

	for i := 0; i < ocs.nbnodes; i++ {
		src, rxMsg, err := receiver.Recv(ctx)
		if err != nil {
			return nil, xerrors.Errorf(unexpectedStreamStop, err)
		}

		dela.Logger.Debug().Msgf("Received a decryption reply from %v", src)

		reply, ok := rxMsg.(types.ReencryptReply)
		if !ok {
			return nil, xerrors.Errorf(unexpectedReply, reply, rxMsg)
		}

		err = processReencryptReply(ocs, &reply)
		if err == nil {
			dela.Logger.Debug().Msgf("Reencryption Uis: %v", ocs.Uis)

			XhatEnc, err := share.RecoverCommit(suites.MustFind("Ed25519"), ocs.Uis, ocs.threshold, ocs.nbnodes)
			if err != nil {
				return nil, xerrors.Errorf("Reencryption failed: %v", err)
			}

			// we have successfully reencrypted
			return XhatEnc, nil
		}
	}

	return nil, xerrors.Errorf("Reencryption failed: %v", err)
}

func processReencryptReply(ocs *onChainSecret, reply *types.ReencryptReply) (err error) {
	if reply.Ui == nil {
		err = xerrors.Errorf("Received empty reply")
		dela.Logger.Warn().Msg("Empty reply received")
		ocs.nbfailures++
		if ocs.nbfailures > ocs.nbnodes-ocs.threshold {
			err = xerrors.Errorf("couldn't get enough shares")
			dela.Logger.Warn().Msg(err.Error())
		}
		return err
	}

	ocs.replies = append(ocs.replies, *reply)

	if len(ocs.replies) >= ocs.threshold {
		ocs.Uis = make([]*share.PubShare, 0, ocs.nbnodes)

		for _, r := range ocs.replies {

			/*
				// Verify proofs
				ufi := suite.Point().Mul(r.Fi, suite.Point().Add(ocs.U, ocs.pubk))
				uiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), r.Ui.V)
				uiHat := suite.Point().Add(ufi, uiei)

				gfi := suite.Point().Mul(r.Fi, nil)
				gxi := ocs.poly.Eval(r.Ui.I).V
				hiei := suite.Point().Mul(suite.Scalar().Neg(r.Ei), gxi)
				hiHat := suite.Point().Add(gfi, hiei)
				hash := sha256.New()
				r.Ui.V.MarshalTo(hash)
				uiHat.MarshalTo(hash)
				hiHat.MarshalTo(hash)
				e := suite.Scalar().SetBytes(hash.Sum(nil))
				if e.Equal(r.Ei) {

			*/
			ocs.Uis = append(ocs.Uis, r.Ui)
			/*
				}
				else
				{
					dela.Logger.Warn().Msgf("Received invalid share from node: %v", r.Ui.I)
					ocs.nbfailures++
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
