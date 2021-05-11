package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/satori/go.uuid"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/contracts/evoting"
	"go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	txnPoolController "go.dedis.ch/dela/core/txn/pool/controller"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/shuffle"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const url = "http://localhost:"

const loginEndPoint = "/evoting/login"
const createElectionEndPoint = "/evoting/create"
const castVoteEndpoint = "/evoting/cast"
const getElectionInfoEndpoint = "/evoting/info"
const getAllElectionsInfoEndpoint = "/evoting/info/all"
const closeElectionEndpoint = "/evoting/close"
const shuffleBallotsEndpoint = "/evoting/shuffle"
const decryptBallotsEndpoint = "/evoting/decrypt"
const getElectionResultEndpoint = "/evoting/result"
const cancelElectionEndpoint = "/evoting/cancel"

const token = "token"
const signerFilePath = "private.key"
const createElectionTimeout = 2 * time.Second

var suite = suites.MustFind("Ed25519")

// TODO : Merge evoting and DKG web server ?

// todo : server should not panic when shuffling 1 ballot

// getManager is the function called when we need a transaction manager. It
// allows us to use a different manager for the tests.
var getManager = func(signer crypto.Signer, s signed.Client) txn.Manager {
	return signed.NewManager(signer, s)
}

// initHttpServer is an action to initialize the shuffle protocol
//
// - implements node.ActionTemplate
type initHttpServerAction struct {
	// TODO : mutex ?

	ElectionIdNonce int
	ElectionIds []string
}

// Execute implements node.ActionTemplate. It implements the handling of endpoints
// and start the HTTP server
func (a *initHttpServerAction) Execute(ctx node.Context) error {
	portNumber := ctx.Flags.String("portNumber")

	http.HandleFunc(loginEndPoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(loginEndPoint)

		userID := uuid.NewV4()
		userToken := token

		response := types.LoginResponse{
			UserID: userID.String(),
			Token:  userToken,
		}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal LoginResponse: " + err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
			return
		}

	})

	http.HandleFunc(createElectionEndPoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(createElectionEndPoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		createElectionRequest := new (types.CreateElectionRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(createElectionRequest)
		if err != nil {
			http.Error(w, "Failed to decode CreateElectionRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if createElectionRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, "Failed to resolve pool.Pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, "Failed to get Signer: " + err.Error(), http.StatusInternalServerError)
			return
		}

		/*
		var client *txnPoolController.Client
		err = ctx.Injector.Resolve(&client)
		if err != nil {
			http.Error(w, "Failed to resolve txn pool controller Client: " + err.Error(), http.StatusInternalServerError)
			return
		}*/

		var blocks *blockstore.InDisk
		err = ctx.Injector.Resolve(&blocks)
		if err != nil {
			http.Error(w, "Failed to resolve blockstore.InDisk: " + err.Error(), http.StatusInternalServerError)
			return
		}

		blockLink, err := blocks.Last()
		if err != nil {
			http.Error(w, "Failed to fetch last block: " + err.Error(), http.StatusInternalServerError)
			return
		}

		transactions := blockLink.GetBlock().GetTransactions()
		nonce := uint64(0)

		for _, tx := range transactions {
			if tx.GetNonce() > nonce{
				nonce = tx.GetNonce()
			}
		}

		nonce += 1
		client := &txnPoolController.Client{Nonce: nonce}

		manager := getManager(signer, client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, "Failed to sync manager: " + err.Error(), http.StatusInternalServerError)
			return
		}

		electionId :=  strconv.Itoa(a.ElectionIdNonce)
		l := len (electionId)
		for i := l; i <32; i++ {
			electionId = "0" + electionId
		}

		createElectionTransaction := types.CreateElectionTransaction{
			ElectionID: electionId,
			Title:      createElectionRequest.Title,
			AdminId:    createElectionRequest.AdminId,
			Candidates: createElectionRequest.Candidates,
			PublicKey:  createElectionRequest.PublicKey,
		}

		js, err := json.Marshal(createElectionTransaction)
		if err != nil {
			http.Error(w, "Failed to marshal CreateElectionTransaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   native.ContractArg,
			Value: []byte(evoting.ContractName),
		}
		args[1] = txn.Arg{
			Key:   evoting.CmdArg,
			Value: []byte(evoting.CmdCreateElection),
		}
		args[2] = txn.Arg{
			Key:   evoting.CreateElectionArg,
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, "Failed to create transaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, "Failed to add transaction to the pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for event := range events {
			for _, res := range event.Transactions {
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Debug().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()
				if !accepted {
					http.Error(w, "Transaction not accepted: " + msg, http.StatusInternalServerError)
					return
				}

				a.ElectionIdNonce+=1
				a.ElectionIds = append(a.ElectionIds, electionId)

				response := types.CreateElectionResponse{
					ElectionID: electionId,
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, "Failed to marshal CreateElectionResponse: " + err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
					return
				}
				return
			}
		}

		http.Error(w, "Transaction not found in the block", http.StatusInternalServerError)
		return
	})

	http.HandleFunc(getElectionInfoEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(getElectionInfoEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		getElectionInfoRequest := new (types.GetElectionInfoRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(getElectionInfoRequest)
		if err != nil {
			http.Error(w, "Failed to decode GetElectionInfoRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if getElectionInfoRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, getElectionInfoRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		proof, err := service.GetProof([]byte(getElectionInfoRequest.ElectionID))
		if err != nil {
			http.Error(w, "Failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		election := new (types.Election)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
		if err != nil {
			http.Error(w, "Failed to unmarshal Election: " + err.Error(), http.StatusInternalServerError)
			return
		}

		response := types.GetElectionInfoResponse{
			Title:      election.Title,
			Candidates: election.Candidates,
			Status:     uint16(election.Status),
			Pubkey:     election.Pubkey,
		}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal GetElectionInfoResponse: " + err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
			return
		}

		return

	})

	http.HandleFunc(getAllElectionsInfoEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(getAllElectionsInfoEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		getAllElectionsInfoRequest := new (types.GetAllElectionsInfoRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(getAllElectionsInfoRequest)
		if err != nil {
			http.Error(w, "Failed to decode GetAllElectionsInfoRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if getAllElectionsInfoRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		allElectionsInfo := make([]types.GetElectionInfoResponse, 0, len(a.ElectionIds))

		for _, id := range a.ElectionIds {

			proof, err := service.GetProof([]byte(id))
			if err != nil {
				http.Error(w, "Failed to read on the blockchain: "+err.Error(), http.StatusInternalServerError)
				return
			}

			election := new(types.Election)
			err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
			if err != nil {
				http.Error(w, "Failed to unmarshal Election: "+err.Error(), http.StatusInternalServerError)
				return
			}

			info := types.GetElectionInfoResponse{
				Title:      election.Title,
				Candidates: election.Candidates,
				Status:     uint16(election.Status),
				Pubkey:     election.Pubkey,
			}

			allElectionsInfo = append(allElectionsInfo, info)
		}

		response := types.GetAllElectionsInfoResponse{AllElectionsInfo: allElectionsInfo}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal GetAllElectionsInfoResponse: " + err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
			return
		}

		return

	})

	http.HandleFunc(castVoteEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(castVoteEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		castVoteRequest := new (types.CastVoteRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(castVoteRequest)
		if err != nil {
			http.Error(w, "Failed to decode CastVoteRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if castVoteRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, castVoteRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, "Failed to resolve pool.Pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, "Failed to get Signer: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var blocks *blockstore.InDisk
		err = ctx.Injector.Resolve(&blocks)
		if err != nil {
			http.Error(w, "Failed to resolve blockstore.InDisk: " + err.Error(), http.StatusInternalServerError)
			return
		}

		blockLink, err := blocks.Last()
		if err != nil {
			http.Error(w, "Failed to fetch last block: " + err.Error(), http.StatusInternalServerError)
			return
		}

		transactions := blockLink.GetBlock().GetTransactions()
		nonce := uint64(0)

		for _, tx := range transactions {
			if tx.GetNonce() > nonce{
				nonce = tx.GetNonce()
			}
		}

		nonce += 1
		client := &txnPoolController.Client{Nonce: nonce}

		manager := getManager(signer, client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, "Failed to sync manager: " + err.Error(), http.StatusInternalServerError)
			return
		}

		castVoteTransaction := types.CastVoteTransaction{
			ElectionID: castVoteRequest.ElectionID,
			UserId:     castVoteRequest.UserId,
			Ballot:     castVoteRequest.Ballot,
		}

		js, err := json.Marshal(castVoteTransaction)
		if err != nil {
			http.Error(w, "Failed to marshal CastVoteTransaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   native.ContractArg,
			Value: []byte(evoting.ContractName),
		}
		args[1] = txn.Arg{
			Key:   evoting.CmdArg,
			Value: []byte(evoting.CmdCastVote),
		}
		args[2] = txn.Arg{
			Key:   evoting.CastVoteArg,
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, "Failed to create transaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, "Failed to add transaction to the pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for event := range events {
			for _, res := range event.Transactions {
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Debug().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()
				if !accepted {
					http.Error(w, "Transaction not accepted: " + msg, http.StatusInternalServerError)
					return
				}

				response := types.CastVoteResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, "Failed to marshal CastVoteResponse: " + err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
					return
				}
				return
			}
		}

		http.Error(w, "Transaction not found in the block", http.StatusInternalServerError)
		return

	})

	http.HandleFunc(closeElectionEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(closeElectionEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		closeElectionRequest := new (types.CloseElectionRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(closeElectionRequest)
		if err != nil {
			http.Error(w, "Failed to decode CloseElectionRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if closeElectionRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, closeElectionRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, "Failed to resolve pool.Pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, "Failed to get Signer: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var blocks *blockstore.InDisk
		err = ctx.Injector.Resolve(&blocks)
		if err != nil {
			http.Error(w, "Failed to resolve blockstore.InDisk: " + err.Error(), http.StatusInternalServerError)
			return
		}

		blockLink, err := blocks.Last()
		if err != nil {
			http.Error(w, "Failed to fetch last block: " + err.Error(), http.StatusInternalServerError)
			return
		}

		transactions := blockLink.GetBlock().GetTransactions()
		nonce := uint64(0)

		for _, tx := range transactions {
			if tx.GetNonce() > nonce{
				nonce = tx.GetNonce()
			}
		}

		nonce += 1
		client := &txnPoolController.Client{Nonce: nonce}

		manager := getManager(signer, client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, "Failed to sync manager: " + err.Error(), http.StatusInternalServerError)
			return
		}

		closeElectionTransaction := types.CloseElectionTransaction{
			ElectionID: closeElectionRequest.ElectionID,
			UserId:     closeElectionRequest.UserId,
		}

		js, err := json.Marshal(closeElectionTransaction)
		if err != nil {
			http.Error(w, "Failed to marshal CloseElectionTransaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   native.ContractArg,
			Value: []byte(evoting.ContractName),
		}
		args[1] = txn.Arg{
			Key:   evoting.CmdArg,
			Value: []byte(evoting.CmdCloseElection),
		}
		args[2] = txn.Arg{
			Key:   evoting.CloseElectionArg,
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, "Failed to create transaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, "Failed to add transaction to the pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for event := range events {
			for _, res := range event.Transactions {
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Debug().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()
				if !accepted {
					http.Error(w, "Transaction not accepted: " + msg, http.StatusInternalServerError)
					return
				}

				response := types.CloseElectionResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, "Failed to marshal CloseElectionResponse: " + err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
					return
				}

				return
			}
		}

		http.Error(w, "Transaction not found in the block", http.StatusInternalServerError)
		return

	})

	http.HandleFunc(shuffleBallotsEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(shuffleBallotsEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		shuffleBallotsRequest := new (types.ShuffleBallotsRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(shuffleBallotsRequest)
		if err != nil {
			http.Error(w, "Failed to decode ShuffleBallotsRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if shuffleBallotsRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, shuffleBallotsRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		proof, err := service.GetProof([]byte(shuffleBallotsRequest.ElectionID))
		if err != nil {
			http.Error(w, "Failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		election := new (types.Election)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
		if err != nil {
			http.Error(w, "Failed to unmarshal Election: " + err.Error(), http.StatusInternalServerError)
			return
		}

		if election.Status != types.Closed{
			http.Error(w, "The election must be closed !", http.StatusUnauthorized)
			return
		}

		if !(len(election.EncryptedBallots) > 1) {
			http.Error(w, "Only one vote has been casted !", http.StatusNotAcceptable)
			return
		}

		// todo : only server node checks this, maybe new transaction that changes the state !
		if election.AdminId != shuffleBallotsRequest.UserId{
			http.Error(w, "Only the admin can shuffle the ballots !", http.StatusUnauthorized)
			return
		}

		addrs := make([]mino.Address, len(shuffleBallotsRequest.Members))
		pubkeys := make([]crypto.PublicKey, len(shuffleBallotsRequest.Members))

		var m mino.Mino
		err = ctx.Injector.Resolve(&m)
		if err != nil {
			http.Error(w, "Failed to resolve mino.Mino: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for i, member := range shuffleBallotsRequest.Members {
			addr, pubkey, err := decodeMember(member.Address, member.PublicKey, m)
			if err != nil {
				http.Error(w, "Failed to decode CollectiveAuthorityMember: " + err.Error(), http.StatusInternalServerError)
				return
			}

			addrs[i] = addr
			pubkeys[i] = pubkey
		}

		collectiveAuthority := authority.New(addrs, pubkeys)

		var shuffleActor shuffle.Actor
		err = ctx.Injector.Resolve(&shuffleActor)
		if err != nil {
			http.Error(w, "Failed to resolve shuffle.Actor: " + err.Error(), http.StatusInternalServerError)
			return
		}

		err = shuffleActor.Shuffle(collectiveAuthority, string(election.ElectionID))

		if err != nil {
			http.Error(w, "Failed to shuffle: " + err.Error(), http.StatusInternalServerError)
			return
		}

		response := types.ShuffleBallotsResponse{
		}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal ShuffleBallotsResponse: " + err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
			return
		}

		return

	})

	http.HandleFunc(decryptBallotsEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(decryptBallotsEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		decryptBallotsRequest := new (types.DecryptBallotsRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(decryptBallotsRequest)
		if err != nil {
			http.Error(w, "Failed to decode DecryptBallotsRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if decryptBallotsRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, decryptBallotsRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		proof, err := service.GetProof([]byte(decryptBallotsRequest.ElectionID))
		if err != nil {
			http.Error(w, "Failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		election := new (types.Election)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
		if err != nil {
			http.Error(w, "Failed to unmarshal Election: " + err.Error(), http.StatusInternalServerError)
			return
		}

		if election.Status != types.ShuffledBallots{
			http.Error(w, "The ballots must have been shuffled !", http.StatusUnauthorized)
			return
		}

		if election.AdminId != decryptBallotsRequest.UserId{
			http.Error(w, "Only the admin can decrypt the ballots !", http.StatusUnauthorized)
			return
		}

		Ks := make([]kyber.Point, 0, len(election.ShuffledBallots))
		Cs := make([]kyber.Point, 0, len(election.ShuffledBallots))


		// todo : PUT THRESHOLD IN ELECTION
		for _, v := range election.ShuffledBallots[3]{
			ciphertext:= new (types.Ciphertext)
			err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
			if err != nil {
				http.Error(w, "Failed to unmarshal Ciphertext: " + err.Error(), http.StatusInternalServerError)
				return
			}

			K := suite.Point()
			err = K.UnmarshalBinary(ciphertext.K)
			if err != nil {
				http.Error(w, "Failed to unmarshal Kyber.Point: " + err.Error(), http.StatusInternalServerError)
				return
			}

			C := suite.Point()
			err = C.UnmarshalBinary(ciphertext.C)
			if err != nil {
				http.Error(w, "Failed to unmarshal Kyber.Point: " + err.Error(), http.StatusInternalServerError)
				return
			}

			Ks = append(Ks, K)
			Cs = append(Cs, C)
		}

		// todo : implement a dkg call to decrypt multiple ciphertexts

		var dkgActor dkg.Actor
		err = ctx.Injector.Resolve(&dkgActor)
		if err != nil {
			http.Error(w, "Failed to resolve dkg.Actor: " + err.Error(), http.StatusInternalServerError)
			return
		}

		decryptedBallots := make([]types.Ballot, 0, len(election.ShuffledBallots))

		for i:=0; i < len(Ks); i++ {
			message, err := dkgActor.Decrypt(Ks[i], Cs[i])
			if err != nil {
				http.Error(w, "Failed to decrypt (K,C): " + err.Error(), http.StatusInternalServerError)
				return
			}

			decryptedBallots = append(decryptedBallots, types.Ballot{Vote: string(message)})
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, "Failed to resolve pool.Pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, "Failed to get Signer: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var blocks *blockstore.InDisk
		err = ctx.Injector.Resolve(&blocks)
		if err != nil {
			http.Error(w, "Failed to resolve blockstore.InDisk: " + err.Error(), http.StatusInternalServerError)
			return
		}

		blockLink, err := blocks.Last()
		if err != nil {
			http.Error(w, "Failed to fetch last block: " + err.Error(), http.StatusInternalServerError)
			return
		}

		transactions := blockLink.GetBlock().GetTransactions()
		nonce := uint64(0)

		for _, tx := range transactions {
			if tx.GetNonce() > nonce{
				nonce = tx.GetNonce()
			}
		}

		nonce += 1
		client := &txnPoolController.Client{Nonce: nonce}

		manager := getManager(signer, client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, "Failed to sync manager: " + err.Error(), http.StatusInternalServerError)
			return
		}

		decryptBallotsTransaction := types.DecryptBallotsTransaction{
			ElectionID:       decryptBallotsRequest.ElectionID,
			UserId:           decryptBallotsRequest.UserId,
			DecryptedBallots: decryptedBallots,
		}

		js, err := json.Marshal(decryptBallotsTransaction)
		if err != nil {
			http.Error(w, "Failed to marshal DecryptBallotsTransaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   native.ContractArg,
			Value: []byte(evoting.ContractName),
		}
		args[1] = txn.Arg{
			Key:   evoting.CmdArg,
			Value: []byte(evoting.CmdDecryptBallots),
		}
		args[2] = txn.Arg{
			Key:   evoting.DecryptBallotsArg,
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, "Failed to create transaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, "Failed to add transaction to the pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for event := range events {
			for _, res := range event.Transactions {
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Debug().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()
				if !accepted {
					http.Error(w, "Transaction not accepted: " + msg, http.StatusInternalServerError)
					return
				}

				response := types.DecryptBallotsResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, "Failed to marshal DecryptBallotsResponse: " + err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
					return
				}
				return
			}
		}

		http.Error(w, "Transaction not found in the block", http.StatusInternalServerError)
		return

	})

	http.HandleFunc(getElectionResultEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(getElectionResultEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		getElectionResultRequest := new (types.GetElectionResultRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(getElectionResultRequest)
		if err != nil {
			http.Error(w, "Failed to decode GetElectionResultRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if getElectionResultRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, getElectionResultRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		proof, err := service.GetProof([]byte(getElectionResultRequest.ElectionID))
		if err != nil {
			http.Error(w, "Failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		election := new (types.Election)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
		if err != nil {
			http.Error(w, "Failed to unmarshal Election: " + err.Error(), http.StatusInternalServerError)
			return
		}

		if election.Status != types.ResultAvailable{
			http.Error(w, "The result is not available.", http.StatusUnauthorized)
			return
		}

		response := types.GetElectionResultResponse{Result: election.DecryptedBallots}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal GetElectionResultResponse: " + err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
			return
		}
		return

	})

	http.HandleFunc(cancelElectionEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(cancelElectionEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read Body: " + err.Error(), http.StatusBadRequest)
			return
		}

		cancelElectionRequest := new (types.CancelElectionRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(cancelElectionRequest)
		if err != nil {
			http.Error(w, "Failed to decode CancelElectionRequest: " + err.Error(), http.StatusBadRequest)
			return
		}

		if cancelElectionRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if !contains(a.ElectionIds, cancelElectionRequest.ElectionID){
			http.Error(w, "The election does not exist", http.StatusNotFound)
			return
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, "Failed to resolve pool.Pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, "Failed to get Signer: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var blocks *blockstore.InDisk
		err = ctx.Injector.Resolve(&blocks)
		if err != nil {
			http.Error(w, "Failed to resolve blockstore.InDisk: " + err.Error(), http.StatusInternalServerError)
			return
		}

		blockLink, err := blocks.Last()
		if err != nil {
			http.Error(w, "Failed to fetch last block: " + err.Error(), http.StatusInternalServerError)
			return
		}

		transactions := blockLink.GetBlock().GetTransactions()
		nonce := uint64(0)

		for _, tx := range transactions {
			if tx.GetNonce() > nonce{
				nonce = tx.GetNonce()
			}
		}

		nonce += 1
		client := &txnPoolController.Client{Nonce: nonce}

		manager := getManager(signer, client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, "Failed to sync manager: " + err.Error(), http.StatusInternalServerError)
			return
		}

		cancelElectionTransaction := types.CancelElectionTransaction{
			ElectionID: cancelElectionRequest.ElectionID,
			UserId:     cancelElectionRequest.UserId,
		}

		js, err := json.Marshal(cancelElectionTransaction)
		if err != nil {
			http.Error(w, "Failed to marshal CancelElectionTransaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   native.ContractArg,
			Value: []byte(evoting.ContractName),
		}
		args[1] = txn.Arg{
			Key:   evoting.CmdArg,
			Value: []byte(evoting.CmdCancelElection),
		}
		args[2] = txn.Arg{
			Key:   evoting.CancelElectionArg,
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, "Failed to create transaction: " + err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, "Failed to resolve ordering.Service: " + err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, "Failed to add transaction to the pool: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for event := range events {
			for _, res := range event.Transactions {
				if !bytes.Equal(res.GetTransaction().GetID(), tx.GetID()) {
					continue
				}

				dela.Logger.Debug().
					Hex("id", tx.GetID()).
					Msg("transaction included in the block")

				accepted, msg := res.GetStatus()
				if !accepted {
					http.Error(w, "Transaction not accepted: " + msg, http.StatusInternalServerError)
					return
				}

				response := types.CancelElectionResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, "Failed to marshal CreateElectionResponse: " + err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, "Failed to write in ResponseWriter: " + err.Error(), http.StatusInternalServerError)
					return
				}
				return
			}
		}

		http.Error(w, "Transaction not found in the block", http.StatusInternalServerError)
		return

	})

	log.Fatal(http.ListenAndServe(":" + portNumber, nil))

	return nil
}

func decodeMember(address string, publicKey string, m mino.Mino) (mino.Address, crypto.PublicKey, error) {

	// 1. Deserialize the address.
	addrBuf, err := base64.StdEncoding.DecodeString(address)
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 address: %v", err)
	}

	addr := m.GetAddressFactory().FromText(addrBuf)

	// 2. Deserialize the public key.
	publicKeyFactory := ed25519.NewPublicKeyFactory()

	pubkeyBuf, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 public key: %v", err)
	}

	pubkey, err := publicKeyFactory.FromBytes(pubkeyBuf)
	if err != nil {
		return nil, nil, xerrors.Errorf("Failed to decode public key: %v", err)
	}

	return addr, pubkey, nil
}


// TODO : the user has to create the file in advance, maybe we should create it here ?
// getSigner creates a signer from a file.
func getSigner(filePath string) (crypto.Signer, error) {
	l := loader.NewFileLoader(filePath)

	signerData, err := l.Load()
	if err != nil {
		return nil, xerrors.Errorf("Failed to load signer: %v", err)
	}

	signer, err := bls.NewSignerFromBytes(signerData)
	if err != nil {
		return nil, xerrors.Errorf("Failed to unmarshal signer: %v", err)
	}

	return signer, nil
}

// createElectionTestAction is an action to
//
// - implements node.ActionTemplate
type createElectionTestAction struct {
}

// Execute implements node.ActionTemplate. It creates
func (a *createElectionTestAction) Execute(ctx node.Context) error {

	createSimpleElectionRequest := types.CreateElectionRequest{
		Title:      "TitleTest",
		AdminId:    "adminId",
		Candidates: nil,
		Token:      "token",
		PublicKey:  nil,
	}

	js, err := json.Marshal(createSimpleElectionRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err := http.Post(url + strconv.Itoa(1000) + createElectionEndPoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))

	return nil
}

// castVoteTestAction is an action to
//
// - implements node.ActionTemplate
type castVoteTestAction struct {
}

// Execute implements node.ActionTemplate. It creates
func (a *castVoteTestAction) Execute(ctx node.Context) error {

	castVoteRequest := types.CastVoteRequest{
		ElectionID: "00000000000000000000000000000000",
		UserId:     "user1",
		Ballot:     []byte("ballot1"),
		Token:      token,
	}

	js, err := json.Marshal(castVoteRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err := http.Post(url + strconv.Itoa(1000) + castVoteEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))

	var service ordering.Service
	err = ctx.Injector.Resolve(&service)
	if err != nil {
		return xerrors.Errorf("failed to resolve service: %v", err)
	}

	proof, err := service.GetProof([]byte(strconv.Itoa(0)))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election := new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshal SimpleElection : %v", err)
	}



	dela.Logger.Info().Msg("Length encrypted ballots : " + strconv.Itoa(len(election.EncryptedBallots)))
	dela.Logger.Info().Msg("Ballot of user1 : " + string(election.EncryptedBallots["user1"]))

	return nil
}

// scenarioTestAction is an action to
//
// - implements node.ActionTemplate
type scenarioTestAction struct {
}

// Execute implements node.ActionTemplate. It creates
func (a *scenarioTestAction) Execute(ctx node.Context) error {

	var service ordering.Service
	err := ctx.Injector.Resolve(&service)
	if err != nil {
		return xerrors.Errorf("failed to resolve service: %v", err)
	}

	var dkgActor dkg.Actor
	err = ctx.Injector.Resolve(&dkgActor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	pubkey, err := dkgActor.GetPublicKey()
	if err != nil {
		return xerrors.Errorf("failed to retrieve the public key: %v", err)
	}

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to encode pubkey: %v", err)
	}


	// ###################################### CREATE SIMPLE ELECTION ###################################################

	dela.Logger.Info().Msg("----------------------- CREATE SIMPLE ELECTION : ")

	createSimpleElectionRequest := types.CreateElectionRequest{
		Title:      "TitleTest",
		AdminId:    "adminId",
		Candidates: nil,
		Token:      "token",
		PublicKey:  pubkeyBuf,
	}

	js, err := json.Marshal(createSimpleElectionRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err := http.Post(url + strconv.Itoa(1000) + createElectionEndPoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err := io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	createSimpleElectionResponse := new(types.CreateElectionResponse)

	err = json.NewDecoder(bytes.NewBuffer(body)).Decode(createSimpleElectionResponse)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshal CastVoteTransaction : %v", err)
	}

	electionId := createSimpleElectionResponse.ElectionID

	proof, err := service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}


	dela.Logger.Info().Msg("Proof : " + string(proof.GetValue()))
	election := new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + election.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Admin Id of the election : " + election.AdminId)
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))

	// ##################################### CREATE SIMPLE ELECTION ####################################################

	// ##################################### GET ELECTION INFO #########################################################

	dela.Logger.Info().Msg("----------------------- GET ELECTION INFO : ")

	getElectionInfoRequest := types.GetElectionInfoRequest{
		ElectionID: electionId,
		Token:      token,
	}

	js, err = json.Marshal(getElectionInfoRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + getElectionInfoEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)
	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election = new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + election.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))
	dela.Logger.Info().Msg("Pubkey of the election : " + string(election.Pubkey))
	dela.Logger.Info().
		Hex("DKG public key", pubkeyBuf).
		Msg("DKG public key")


	// ##################################### GET ELECTION INFO #########################################################

	// ##################################### CAST BALLOTS ##############################################################

	dela.Logger.Info().Msg("----------------------- CAST BALLOTS : ")

	ballot1, err := marshallBallot("ballot1", dkgActor)
	if err != nil {
		return xerrors.Errorf("failed to marshall ballot : %v", err)
	}

	ballot2, err := marshallBallot("ballot2", dkgActor)
	if err != nil {
		return xerrors.Errorf("failed to marshall ballot : %v", err)
	}

	ballot3, err := marshallBallot("ballot3", dkgActor)
	if err != nil {
		return xerrors.Errorf("failed to marshall ballot : %v", err)
	}

	castVoteRequest := types.CastVoteRequest{
		ElectionID: electionId,
		UserId:     "user1",
		Ballot:     ballot1,
		Token:      token,
	}

	js, err = json.Marshal(castVoteRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + castVoteEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)
	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	castVoteRequest = types.CastVoteRequest{
		ElectionID: electionId,
		UserId:     "user2",
		Ballot:     ballot2,
		Token:      token,
	}

	js, err = json.Marshal(castVoteRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + castVoteEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)
	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	castVoteRequest = types.CastVoteRequest{
		ElectionID: electionId,
		UserId:     "user3",
		Ballot:     ballot3,
		Token:      token,
	}

	js, err = json.Marshal(castVoteRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + castVoteEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)
	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()


	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election = new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshal SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Length encrypted ballots : " + strconv.Itoa(len(election.EncryptedBallots)))
	dela.Logger.Info().Msg("Ballot of user1 : " + string(election.EncryptedBallots["user1"]))
	dela.Logger.Info().Msg("Ballot of user2 : " + string(election.EncryptedBallots["user2"]))
	dela.Logger.Info().Msg("Ballot of user3 : " + string(election.EncryptedBallots["user3"]))
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))

	// ##################################### CAST BALLOTS ##############################################################

	// ###################################### CLOSE ELECTION ###########################################################

	dela.Logger.Info().Msg("----------------------- CLOSE ELECTION : ")

	closeElectionRequest := types.CloseElectionRequest{
		ElectionID: electionId,
		UserId:     "adminId",
		Token:      token,
	}

	js, err = json.Marshal(closeElectionRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + closeElectionEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election = new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + election.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Admin Id of the election : " + election.AdminId)
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))

	// ##################################### CLOSE ELECTION ############################################################

	// ###################################### SHUFFLE BALLOTS ##########################################################

	dela.Logger.Info().Msg("----------------------- SHUFFLE BALLOTS : ")

	roster, err := a.readMembers(ctx)
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	shuffleBallotsRequest := types.ShuffleBallotsRequest{
		ElectionID: electionId,
		UserId:     "adminId",
		Token:      token,
		Members:    roster,
	}

	js, err = json.Marshal(shuffleBallotsRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + shuffleBallotsEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	time.Sleep(20 * time.Second)

	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election = new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + election.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))
	dela.Logger.Info().Msg("Number of shuffled ballots : " + strconv.Itoa(len(election.ShuffledBallots)))
	dela.Logger.Info().Msg("Number of encrypted ballots : " + strconv.Itoa(len(election.EncryptedBallots)))

	// ###################################### SHUFFLE BALLOTS ##########################################################

	// ###################################### DECRYPT BALLOTS ##########################################################

	dela.Logger.Info().Msg("----------------------- DECRYPT BALLOTS : ")


	decryptBallotsRequest := types.DecryptBallotsRequest{
		ElectionID: electionId,
		UserId:     "adminId",
		Token:      token,
	}

	js, err = json.Marshal(decryptBallotsRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + decryptBallotsEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election = new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("----------------------- Election : " + string(proof.GetValue()))
	dela.Logger.Info().Msg("Title of the election : " + election.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))
	dela.Logger.Info().Msg("Number of decrypted ballots : " + strconv.Itoa(len(election.DecryptedBallots)))
	//dela.Logger.Info().Msg("decrypted ballots [0] : " + election.DecryptedBallots[0].Vote)
	//dela.Logger.Info().Msg("decrypted ballots [1] : " + election.DecryptedBallots[1].Vote)
	//dela.Logger.Info().Msg("decrypted ballots [2] : " + election.DecryptedBallots[2].Vote)


	// ###################################### DECRYPT BALLOTS ##########################################################

	// ###################################### GET ELECTION RESULT ######################################################


	dela.Logger.Info().Msg("----------------------- GET ELECTION RESULT : ")


	getElectionResultRequest := types.GetElectionResultRequest{
		ElectionID: electionId,
		Token:      token,
	}

	js, err = json.Marshal(getElectionResultRequest)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	resp, err = http.Post(url + strconv.Itoa(1000) + getElectionResultEndpoint, "application/json", bytes.NewBuffer(js))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	body, err = io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))
	resp.Body.Close()

	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	election = new (types.Election)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + election.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(election.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(election.Status)))
	dela.Logger.Info().Msg("Number of decrypted ballots : " + strconv.Itoa(len(election.DecryptedBallots)))
	//dela.Logger.Info().Msg("decrypted ballots [0] : " + election.DecryptedBallots[0].Vote)
	//dela.Logger.Info().Msg("decrypted ballots [1] : " + election.DecryptedBallots[1].Vote)
	//dela.Logger.Info().Msg("decrypted ballots [2] : " + election.DecryptedBallots[2].Vote)


	// ###################################### GET ELECTION RESULT ######################################################

	return nil
}

func marshallBallot (vote string, actor dkg.Actor) ([]byte, error) {

	K, C, _, err := actor.Encrypt([]byte(vote))
	if err != nil {
		return nil, xerrors.Errorf("failed to encrypt the plaintext: %v", err)
	}

	Kmarshalled, err := K.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshall the K element of the ciphertext pair: %v", err)
	}

	Cmarshalled, err := C.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshall the C element of the ciphertext pair: %v", err)
	}

	ballot := types.Ciphertext{K: Kmarshalled, C: Cmarshalled}
	js, err := json.Marshal(ballot)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshall Ciphertext: %v", err)
	}

	return js, nil

}

func (a scenarioTestAction) readMembers(ctx node.Context) ([]types.CollectiveAuthorityMember, error) {
	members := ctx.Flags.StringSlice("member")

	roster := make([]types.CollectiveAuthorityMember, len(members))

	for i, member := range members {
		addr, pubkey, err := decodeMemberFromContext(member)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode: %v", err)
		}

		roster[i] = types.CollectiveAuthorityMember{
			Address:   addr,
			PublicKey: pubkey,
		}
	}

	return roster, nil
}

func decodeMemberFromContext(str string) (string, string, error) {
	parts := strings.Split(str, ":")
	if len(parts) != 2 {
		return "", "", xerrors.New("invalid member base64 string")
	}

	return parts[0], parts[1], nil
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
