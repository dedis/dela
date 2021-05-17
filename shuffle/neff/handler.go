package neff

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/contracts/evoting"
	electionTypes "go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	txnPoolController "go.dedis.ch/dela/core/txn/pool/controller"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/shuffle/neff/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/proof"
	shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"strconv"
	"strings"
	"time"
)

const shuffleTransactionTimeout = time.Second * 2

var suite = suites.MustFind("Ed25519")

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	me      mino.Address
	service ordering.Service
	p       pool.Pool
	blocks  blockstore.BlockStore
	signer  crypto.Signer
}

// NewHandler creates a new handler
func NewHandler(me mino.Address, service ordering.Service, p pool.Pool, blocks blockstore.BlockStore, signer crypto.Signer) *Handler {
	return &Handler{
		me:      me,
		service: service,
		p:       p,
		blocks:  blocks,
		signer:  signer,
	}
}

// Stream implements mino.Handler. It allows one to stream messages to the
// players.
func (h *Handler) Stream(out mino.Sender, in mino.Receiver) error {

	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return xerrors.Errorf("failed to receive: %v", err)
	}

	switch msg := msg.(type) {

	case types.StartShuffle:
		err := h.HandleStartShuffleMessage(msg, from, out, in)
		if err != nil {
			return xerrors.Errorf("failed to handle StartShuffle message: %v", err)
		}

	default:
		return xerrors.Errorf("expected StartShuffle message, got: %T", msg)
	}

	return nil
}

func (h *Handler) HandleStartShuffleMessage(startShuffleMessage types.StartShuffle, from mino.Address, out mino.Sender,
	in mino.Receiver) error {

	dela.Logger.Info().Msg("Starting the neff shuffle protocol ...")

	dela.Logger.Info().Msg("SHUFFLE / RECEIVED FROM  : " + from.String())

	for round := 1; round <= startShuffleMessage.GetThreshold(); round++ {
		dela.Logger.Info().Msg("SHUFFLE / ROUND : " + strconv.Itoa(round))

		electionIDBuff, _ := hex.DecodeString(startShuffleMessage.GetElectionId())

		prf, err := h.service.GetProof(electionIDBuff)
		if err != nil {
			return xerrors.Errorf("failed to read on the blockchain: %v", err)
		}

		election := new(electionTypes.Election)

		err = json.NewDecoder(bytes.NewBuffer(prf.GetValue())).Decode(election)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal Election: %v", err)
		}

		if election.Status != electionTypes.Closed {
			return xerrors.Errorf("the election must be closed")
		}

		encryptedBallotsMap := election.EncryptedBallots

		encryptedBallots := make([][]byte, 0, len(encryptedBallotsMap))

		if round == 1 {
			for _, value := range encryptedBallotsMap {
				encryptedBallots = append(encryptedBallots, value)
			}
		}

		if round > 1 {
			encryptedBallots = election.ShuffledBallots[round-1]
		}

		Ks := make([]kyber.Point, 0, len(encryptedBallotsMap))
		Cs := make([]kyber.Point, 0, len(encryptedBallotsMap))

		for _, v := range encryptedBallots {
			ciphertext := new(electionTypes.Ciphertext)
			err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
			if err != nil {
				return xerrors.Errorf("failed to unmarshal Ciphertext: %v", err)
			}

			K := suite.Point()
			err = K.UnmarshalBinary(ciphertext.K)
			if err != nil {
				return xerrors.Errorf("failed to unmarshal K: %v", err)
			}

			C := suite.Point()
			err = C.UnmarshalBinary(ciphertext.C)
			if err != nil {
				return xerrors.Errorf("failed to unmarshal C: %v", err)
			}

			Ks = append(Ks, K)
			Cs = append(Cs, C)
		}

		pubKey := suite.Point()
		err = pubKey.UnmarshalBinary(election.Pubkey)
		if err != nil {
			return xerrors.Errorf("couldn't unmarshal public key: %v", err)
		}

		rand := suite.RandomStream()
		Kbar, Cbar, prover := shuffleKyber.Shuffle(suite, nil, pubKey, Ks, Cs, rand)
		shuffleProof, err := proof.HashProve(suite, protocolName, prover)
		if err != nil {
			return xerrors.Errorf("Shuffle proof failed: %v", err.Error())
		}

		shuffledBallots := make([][]byte, 0, len(Kbar))

		for i := 0; i < len(Kbar); i++ {

			kMarshalled, err := Kbar[i].MarshalBinary()
			if err != nil {
				return xerrors.Errorf("failed to marshall kyber.Point: %v", err.Error())
			}

			cMarshalled, err := Cbar[i].MarshalBinary()
			if err != nil {
				return xerrors.Errorf("failed to marshall kyber.Point: %v", err.Error())
			}

			ciphertext := electionTypes.Ciphertext{K: kMarshalled, C: cMarshalled}
			js, err := json.Marshal(ciphertext)
			if err != nil {
				return xerrors.Errorf("failed to marshall Ciphertext: %v", err.Error())
			}

			shuffledBallots = append(shuffledBallots, js)

		}

		client, err := h.getClient()
		if err != nil {
			return xerrors.Errorf("failed to get Client: %v", err.Error())
		}

		manager := getManager(h.signer, client)

		err = manager.Sync()
		if err != nil {
			return xerrors.Errorf("failed to sync manager: %v", err.Error())
		}

		shuffleBallotsTransaction := electionTypes.ShuffleBallotsTransaction{
			ElectionID:      startShuffleMessage.GetElectionId(),
			Round:           round,
			ShuffledBallots: shuffledBallots,
			Proof:           shuffleProof,
		}

		js, err := json.Marshal(shuffleBallotsTransaction)
		if err != nil {
			return xerrors.Errorf("failed to marshal ShuffleBallotsTransaction: %v", err.Error())
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   native.ContractArg,
			Value: []byte(evoting.ContractName),
		}
		args[1] = txn.Arg{
			Key:   evoting.CmdArg,
			Value: []byte(evoting.CmdShuffleBallots),
		}
		args[2] = txn.Arg{
			Key:   evoting.ShuffleBallotsArg,
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			return xerrors.Errorf("failed to make transaction: %v", err.Error())
		}
		dela.Logger.Info().Msg("TRANSACTION NONCE : " + strconv.Itoa(int(tx.GetNonce())))
		watchCtx, cancel := context.WithTimeout(context.Background(), shuffleTransactionTimeout)

		err = h.p.Add(tx)

		events := h.service.Watch(watchCtx)

		// err = h.p.Add(tx)
		if err != nil {
			cancel()
			return xerrors.Errorf("failed to add transaction to the pool: %v", err.Error())
		}
		notAccepted := false

		for event := range events {
			for _, res := range event.Transactions {
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Info().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()

				if !accepted {
					notAccepted = true
					dela.Logger.Info().Msg("Denied : " + msg)
					break
				} else {
					dela.Logger.Info().Msg("ACCEPTED")

					if round == startShuffleMessage.GetThreshold() {
						message := types.EndShuffle{}
						addrs := make([]mino.Address, 0, len(startShuffleMessage.GetAddresses())-1)
						for _, addr := range startShuffleMessage.GetAddresses() {
							if !(addr.Equal(h.me)) {
								addrs = append(addrs, addr)
							}
						}
						errs := out.Send(message, addrs...)
						err = <-errs
						if err != nil {
							cancel()
							return xerrors.Errorf("failed to send EndShuffle message: %v", err)
						}
						dela.Logger.Info().Msg("SENT END SHUFFLE MESSAGES")
					} else {
						dela.Logger.Info().Msg("WAITING FOR END SHUFFLE MESSAGE")
						addr, msg, err := in.Recv(context.Background())
						if err != nil {
							cancel()
							return xerrors.Errorf("got an error from '%s' while "+
								"receiving: %v", addr, err)
						}
						_, ok := msg.(types.EndShuffle)
						if !ok {
							cancel()
							return xerrors.Errorf("expected to receive an EndShuffle message, but "+
								"go the following: %T", msg)
						}
						dela.Logger.Info().Msg("RECEIVED END SHUFFLE MESSAGE")
					}

					if startShuffleMessage.GetAddresses()[0].Equal(h.me) {
						message := types.EndShuffle{}
						errs := out.Send(message, from)
						err = <-errs
						if err != nil {
							cancel()
							return xerrors.Errorf("failed to send EndShuffle message: %v", err)
						}
					}
					cancel()
					return nil
				}
			}
			if notAccepted {
				break
			}
		}
		cancel()
		dela.Logger.Info().Msg("NEXT ROUND")
		continue
	}

	return xerrors.Errorf("failed to shuffle, all your transactions got denied")
}

func (h *Handler) getClient() (*txnPoolController.Client, error) {

	blockLink, err := h.blocks.Last()
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch last block: %v", err.Error())
	}

	transactionResults := blockLink.GetBlock().GetData().GetTransactionResults()
	nonce := uint64(0)

	for nonce == 0 {
		for _, txResult := range transactionResults {
			status, _ := txResult.GetStatus()
			if status && txResult.GetTransaction().GetNonce() > nonce {
				nonce = txResult.GetTransaction().GetNonce()
			}
		}

		previousDigest := blockLink.GetFrom()

		previousBlock, err := h.blocks.Get(previousDigest)
		if err != nil {
			if strings.Contains(err.Error(), "not found: no block") {
				dela.Logger.Info().Msg("FIRST BLOCK")
				break
			} else {
				return nil, xerrors.Errorf("failed to fetch previous block: %v", err)
			}
		} else {
			transactionResults = previousBlock.GetBlock().GetData().GetTransactionResults()
		}
	}

	nonce += 1

	client := &txnPoolController.Client{Nonce: nonce}

	return client, nil
}

// getManager is the function called when we need a transaction manager. It
// allows us to use a different manager for the tests.
var getManager = func(signer crypto.Signer, s signed.Client) txn.Manager {
	return signed.NewManager(signer, s)
}
