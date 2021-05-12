package neff

import (
	"bytes"
	"context"
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
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/shuffle/neff/types"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/proof"
	shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"strconv"
	"sync"
	"time"
)

const shuffleTransactionTimeout = time.Second * 2

var suite = suites.MustFind("Ed25519")

const signerFilePath = "private.key"

// Todo : either add some sort of setup call to set participants or public key
//  (and thus reduce message size ) or remove state
// state is a struct contained in a handler that allows an actor to read the
// state of that handler. The actor should only use the getter functions to read
// the attributes.
type state struct {
	sync.Mutex
	// participants []mino.Address
}

// Handler represents the RPC executed on each node
//
// - implements mino.Handler
type Handler struct {
	mino.UnsupportedHandler
	sync.RWMutex
	me       mino.Address
	startRes *state
	service  ordering.Service
	p        pool.Pool
	blocks   *blockstore.InDisk
}

// NewHandler creates a new handler
func NewHandler(me mino.Address, service ordering.Service, p pool.Pool, blocks *blockstore.InDisk) *Handler {
	return &Handler{
		me:       me,
		startRes: &state{},
		service:  service,
		p:        p,
		blocks:   blocks,
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

	case types.ShuffleMessage:
		err := h.HandleShuffleMessage(msg, from, out, in)
		if err != nil {
			return xerrors.Errorf("failed to handle ShuffleMessage: %v", err)
		}

	default:
		return xerrors.Errorf("expected Start message, decrypt request or "+
			"Deal as first message, got: %T", msg)
	}

	return nil
}

func (h *Handler) HandleStartShuffleMessage(startShuffleMessage types.StartShuffle, from mino.Address, out mino.Sender,
	in mino.Receiver) error {

	dela.Logger.Info().Msg("Starting the neff shuffle protocol ...")

	dela.Logger.Info().Msg("SHUFFLE / RECEIVED FROM  : " + from.String())

	signer, err := getSigner(signerFilePath)
	if err != nil {
		return xerrors.Errorf("failed to getSigner: %v", err)
	}

	for round := 1; round <= startShuffleMessage.GetThreshold(); round++ {
		dela.Logger.Info().Msg("SHUFFLE / ROUND : " + strconv.Itoa(round))

		prf, err := h.service.GetProof([]byte(startShuffleMessage.GetElectionId()))
		if err != nil {
			return xerrors.Errorf("failed to read on the blockchain: %v", err)
		}

		election := new(electionTypes.Election)

		err = json.NewDecoder(bytes.NewBuffer(prf.GetValue())).Decode(election)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal Election: %v", err)
		}

		if election.Status != electionTypes.Closed {
			return xerrors.Errorf("The election must be closed !")
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

		blockLink, err := h.blocks.Last()
		if err != nil {
			return xerrors.Errorf("failed to fetch last block: %v", err.Error())
		}

		transactionResults := blockLink.GetBlock().GetData().GetTransactionResults()
		nonce := uint64(0)

		for _, txResult := range transactionResults {
			status, _ := txResult.GetStatus()
			if status && txResult.GetTransaction().GetNonce() > nonce {
				nonce = txResult.GetTransaction().GetNonce()
			}
		}

		nonce += 1
		client := &txnPoolController.Client{Nonce: nonce}
		manager := getManager(signer, client)

		err = manager.Sync()
		if err != nil {
			return xerrors.Errorf("failed to sync manager: %v", err.Error())
		}

		shuffleBallotsTransaction := electionTypes.ShuffleBallotsTransaction{
			ElectionID:      startShuffleMessage.GetElectionId(),
			Round:           round,
			ShuffledBallots: shuffledBallots,
			Proof:           shuffleProof,
			Node:            h.me.String(),
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

		events := h.service.Watch(watchCtx)

		err = h.p.Add(tx)
		if err != nil {
			cancel()
			return xerrors.Errorf("failed to add transaction to the pool: %v", err.Error())
		}

		notAccepted := false

		for event := range events {
			dela.Logger.Info().Msg("iterating events...")
			for _, res := range event.Transactions {
				dela.Logger.Info().Msg("iterating transactions...")
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Info().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()

				if !accepted {
					notAccepted = true
					dela.Logger.Info().Msg("NOT ACCEPTED : " + msg)
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

		// cancel()
		// return xerrors.Errorf("Transaction not found in the block")
	}

	// Todo : think about this !! should not reach this code
	return xerrors.Errorf("Weird, should be unreachable")
}

// Todo : handle edge cases
func (h *Handler) HandleShuffleMessage(shuffleMessage types.ShuffleMessage, from mino.Address, out mino.Sender,
	in mino.Receiver) error {

	dela.Logger.Info().Msg("SHUFFLE / RECEIVED FROM  : " + from.String())

	addrs := shuffleMessage.GetAddresses()
	suite := suites.MustFind(shuffleMessage.GetSuiteName())
	publicKey := shuffleMessage.GetPublicKey()
	kBar := shuffleMessage.GetkBar()
	cBar := shuffleMessage.GetcBar()
	kBarPrevious := shuffleMessage.GetkBarPrevious()
	cBarPrevious := shuffleMessage.GetcBarPrevious()
	prf := shuffleMessage.GetProof()

	// leader node
	if addrs[0].Equal(h.me) {
		dela.Logger.Info().Msg("SHUFFLE / SENDING TO : " + addrs[1].String())
		errs := out.Send(shuffleMessage, addrs[1])
		err := <-errs
		if err != nil {
			return xerrors.Errorf("failed to send Shuffle Message: %v", err)
		}

		lastNodeAddress, msg, err := in.Recv(context.Background())
		if err != nil {
			return xerrors.Errorf("failed to receive msg: %v", err)
		}
		dela.Logger.Info().Msg("RECEIVED FROM  : " + lastNodeAddress.String())
		errs = out.Send(msg, from)
		err = <-errs
		if err != nil {
			return xerrors.Errorf("failed to send Shuffle Message: %v", err)
		}
		return nil
	}

	// Todo : check you received from the correct node

	err := verify(suite, kBarPrevious, cBarPrevious, publicKey, kBar, cBar, prf)
	if err != nil {
		return xerrors.Errorf("Shuffle verify failed: %v", err)
	}

	rand := suite.RandomStream()
	KbarNext, CbarNext, prover := shuffleKyber.Shuffle(suite, nil, publicKey, kBar, cBar, rand)
	prfNext, err := proof.HashProve(suite, protocolName, prover)
	if err != nil {
		return xerrors.Errorf("Shuffle proof failed: %v", err)
	}

	message := types.NewShuffleMessage(addrs, shuffleMessage.GetSuiteName(), publicKey, KbarNext,
		CbarNext, kBar, cBar, prfNext)

	index := 0
	for i, addr := range addrs {
		if addr.Equal(from) {
			index = i
			break
		}
	}
	// todo : use modulo
	index += 2

	if index >= len(addrs) {
		index = 0
	}

	dela.Logger.Info().Msg("SHUFFLE / SENDING TO : " + addrs[index].String())

	errs := out.Send(message, addrs[index])
	err = <-errs
	if err != nil {
		return xerrors.Errorf("failed to send Shuffle Message: %v", err)
	}

	return nil
}

func verify(suite suites.Suite, Ks []kyber.Point, Cs []kyber.Point,
	pubKey kyber.Point, KsShuffled []kyber.Point, CsShuffled []kyber.Point, prf []byte) (err error) {

	verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)
	return proof.HashVerify(suite, protocolName, verifier, prf)
}

// TODO : the user has to create the file in advance, maybe we should create it here ?
// getSigner creates a signer from a file.
func getSigner(filePath string) (crypto.Signer, error) {
	l := loader.NewFileLoader(filePath)

	signerData, err := l.Load()
	if err != nil {
		return nil, xerrors.Errorf("failed to load signer: %v", err)
	}

	signer, err := bls.NewSignerFromBytes(signerData)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal signer: %v", err)
	}

	return signer, nil
}

// getManager is the function called when we need a transaction manager. It
// allows us to use a different manager for the tests.
var getManager = func(signer crypto.Signer, s signed.Client) txn.Manager {
	return signed.NewManager(signer, s)
}
