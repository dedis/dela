package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/satori/go.uuid"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
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
const getAllElectionsInfoEndpoint = "/evoting/all"
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
	client *client
}

// Todo : all types should be in another file
type LoginResponse struct {
	UserID string
	Token string
}

type CreateSimpleElectionRequest struct {
	Title string
	AdminId string
	Candidates []string
	Token string
	PublicKey []byte
}

type CreateSimpleElectionTransaction struct {
	ElectionID string
	Title string
	AdminId string
	Candidates []string
	PublicKey []byte
}

type CreateSimpleElectionResponse struct {
	ElectionID string
	//Success bool
	//Error string
}

type CastVoteRequest struct {
	ElectionID string
	UserId string
	Ballot []byte
	Token string
}

type CastVoteTransaction struct {
	ElectionID string
	UserId string
	Ballot []byte
}

type CastVoteResponse struct {
	//Success bool
	//Error string
}

type CollectiveAuthorityMember struct {
	Address string
	PublicKey string
}

// Wraps the ciphertext pairs
type Ciphertext struct {
	K []byte
	C []byte
}

type CloseElectionRequest struct {
	ElectionID string
	UserId string
	Token string
}

type CloseElectionTransaction struct {
	ElectionID string
	UserId string
}

type CloseElectionResponse struct {
	//Success bool
	//Error string
}

type ShuffleBallotsRequest struct {
	ElectionID string
	UserId string
	Token string
	Members []CollectiveAuthorityMember
}

type ShuffleBallotsTransaction struct {
	ElectionID string
	UserId string
	ShuffledBallots  [][]byte
	Proof 			 []byte
}

type ShuffleBallotsResponse struct {
	//Success bool
	//Error string
}

type DecryptBallotsRequest struct {
	ElectionID string
	UserId string
	Token string
}

type DecryptBallotsTransaction struct {
	ElectionID string
	UserId string
	DecryptedBallots []types.SimpleBallot
}

type DecryptBallotsResponse struct {
	//Success bool
	//Error string
}

type GetElectionResultRequest struct {
	ElectionID string
	//UserId string
	Token string
}

type GetElectionResultResponse struct {
	Result  []types.SimpleBallot
	//Success bool
	//Error string
}

type GetElectionInfoRequest struct {
	ElectionID string
	//UserId string
	Token string
}

type GetElectionInfoResponse struct {
	ElectionID string
	Title            string
	Candidates       []string
	Status           uint16
	Pubkey           []byte
	Result  []types.SimpleBallot
	//Success bool
	//Error string
}

type GetAllElectionsInfoRequest struct {
	//UserId string
	Token string
}

type GetAllElectionsInfoResponse struct {
	//UserId string
	AllElectionsInfo []GetElectionInfoResponse
}


type CancelElectionRequest struct {
	ElectionID string
	UserId string
	Token string
}

type CancelElectionTransaction struct {
	ElectionID string
	UserId string
}

type CancelElectionResponse struct {
	//Success bool
	//Error string
}

// Execute implements node.ActionTemplate. It implements the handling of endpoints
// and start the HTTP server
func (a *initHttpServerAction) Execute(ctx node.Context) error {
	portNumber := ctx.Flags.String("portNumber")

	http.HandleFunc(loginEndPoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(loginEndPoint)

		userID := uuid.NewV4()
		userToken := token

		response := LoginResponse{
			UserID: userID.String(),
			Token:  userToken,
		}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	})

	http.HandleFunc(createElectionEndPoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(createElectionEndPoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		createSimpleElectionRequest := new (CreateSimpleElectionRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(createSimpleElectionRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if createSimpleElectionRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager := getManager(signer, a.client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		electionId :=  strconv.Itoa(a.ElectionIdNonce)
		l := len (electionId)
		for i := l; i <32; i++ {
			electionId = "0" + electionId
		}

		createSimpleElectionTransaction := CreateSimpleElectionTransaction{
			ElectionID: electionId,
			Title:      createSimpleElectionRequest.Title,
			AdminId:    createSimpleElectionRequest.AdminId,
			Candidates: createSimpleElectionRequest.Candidates,
			PublicKey:  createSimpleElectionRequest.PublicKey,
		}

		js, err := json.Marshal(createSimpleElectionTransaction)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   "go.dedis.ch/dela.ContractArg",
			Value: []byte("go.dedis.ch/dela.Evoting"),
		}
		args[1] = txn.Arg{
			Key:   "evoting:command",
			Value: []byte("CREATE_SIMPLE_ELECTION"),
		}
		args[2] = txn.Arg{
			Key:   "evoting:simpleElectionArgs",
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
					http.Error(w, "Transaction refused : " + msg, http.StatusInternalServerError)
					return
				}

				a.ElectionIdNonce+=1
				a.ElectionIds = append(a.ElectionIds, electionId)

				response := CreateSimpleElectionResponse{
					ElectionID: electionId,
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		getElectionInfoRequest := new (GetElectionInfoRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(getElectionInfoRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		proof, err := service.GetProof([]byte(getElectionInfoRequest.ElectionID))
		if err != nil {
			http.Error(w, "failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		simpleElection := new (types.SimpleElection)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
		if err != nil {
			http.Error(w, "failed to unmarshall SimpleElection: " + err.Error(), http.StatusInternalServerError)
			return
		}

		response := GetElectionInfoResponse{
			Title:      simpleElection.Title,
			Candidates: simpleElection.Candidates,
			Status:     uint16(simpleElection.Status),
			Pubkey:     simpleElection.Pubkey,
			Result: simpleElection.DecryptedBallots,
		}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return

	})

	http.HandleFunc(getAllElectionsInfoEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(getAllElectionsInfoEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		getAllElectionsInfoRequest := new (GetAllElectionsInfoRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(getAllElectionsInfoRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if getAllElectionsInfoRequest.Token != token{
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		allElectionsInfo := make([]GetElectionInfoResponse, 0, len(a.ElectionIds))

		for _, id := range a.ElectionIds {

			proof, err := service.GetProof([]byte(id))
			if err != nil {
				http.Error(w, "failed to read on the blockchain: "+err.Error(), http.StatusInternalServerError)
				return
			}

			simpleElection := new(types.SimpleElection)
			err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
			if err != nil {
				http.Error(w, "failed to unmarshall SimpleElection: "+err.Error(), http.StatusInternalServerError)
				return
			}

			info := GetElectionInfoResponse{
				ElectionID: string(simpleElection.ElectionID),
				Title:      simpleElection.Title,
				Candidates: simpleElection.Candidates,
				Status:     uint16(simpleElection.Status),
				Pubkey:     simpleElection.Pubkey,
				Result: simpleElection.DecryptedBallots,
			}

			allElectionsInfo = append(allElectionsInfo, info)
		}

		response := GetAllElectionsInfoResponse{AllElectionsInfo: allElectionsInfo}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		return

	})

	http.HandleFunc(castVoteEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(castVoteEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		castVoteRequest := new (CastVoteRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(castVoteRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager := getManager(signer, a.client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		castVoteTransaction := CastVoteTransaction{
			ElectionID: castVoteRequest.ElectionID,
			UserId:     castVoteRequest.UserId,
			Ballot:     castVoteRequest.Ballot,
		}

		js, err := json.Marshal(castVoteTransaction)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   "go.dedis.ch/dela.ContractArg",
			Value: []byte("go.dedis.ch/dela.Evoting"),
		}
		args[1] = txn.Arg{
			Key:   "evoting:command",
			Value: []byte("CAST_VOTE"),
		}
		args[2] = txn.Arg{
			Key:   "evoting:castVoteArgs",
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
					http.Error(w, "Transaction refused : " + msg, http.StatusInternalServerError)
					return
				}

				response := CastVoteResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		closeElectionRequest := new (CloseElectionRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(closeElectionRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager := getManager(signer, a.client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		closeElectionTransaction := CloseElectionTransaction{
			ElectionID: closeElectionRequest.ElectionID,
			UserId:     closeElectionRequest.UserId,
		}

		js, err := json.Marshal(closeElectionTransaction)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   "go.dedis.ch/dela.ContractArg",
			Value: []byte("go.dedis.ch/dela.Evoting"),
		}
		args[1] = txn.Arg{
			Key:   "evoting:command",
			Value: []byte("CLOSE_ELECTION"),
		}
		args[2] = txn.Arg{
			Key:   "evoting:closeElectionArgs",
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
					http.Error(w, "Transaction refused : " + msg, http.StatusInternalServerError)
					return
				}
/*
				var dkgActor dkg.Actor
				err = ctx.Injector.Resolve(&dkgActor)
				if err != nil {
					http.Error(w, "failed to resolve dkg.Actor: " + err.Error(), http.StatusInternalServerError)
					return
				}

				*/

				response := CloseElectionResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		shuffleBallotsRequest := new (ShuffleBallotsRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(shuffleBallotsRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		proof, err := service.GetProof([]byte(shuffleBallotsRequest.ElectionID))
		if err != nil {
			http.Error(w, "failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		simpleElection := new (types.SimpleElection)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
		if err != nil {
			http.Error(w, "failed to unmarshall SimpleElection: " + err.Error(), http.StatusInternalServerError)
			return
		}

		if simpleElection.Status != types.Closed{
			http.Error(w, "The election must be closed !", http.StatusUnauthorized)
			return
		}

		if simpleElection.AdminId != shuffleBallotsRequest.UserId{
			http.Error(w, "Only the admin can shuffle the ballots !", http.StatusUnauthorized)
			return
		}

		addrs := make([]mino.Address, len(shuffleBallotsRequest.Members))
		pubkeys := make([]crypto.PublicKey, len(shuffleBallotsRequest.Members))

		var m mino.Mino
		err = ctx.Injector.Resolve(&m)
		if err != nil {
			http.Error(w, "failed to resolve mino.Mino: " + err.Error(), http.StatusInternalServerError)
			return
		}

		for i, member := range shuffleBallotsRequest.Members {
			addr, pubkey, err := decodeMember(member.Address, member.PublicKey, m)
			if err != nil {
				http.Error(w, "failed to decode collectiveAuthority member: " + err.Error(), http.StatusInternalServerError)
				return
			}

			addrs[i] = addr
			pubkeys[i] = pubkey
		}

		collectiveAuthority := authority.New(addrs, pubkeys)

		encryptedBallotsMap := simpleElection.EncryptedBallots
		Ks := make([]kyber.Point, 0, len(encryptedBallotsMap))
		Cs := make([]kyber.Point, 0, len(encryptedBallotsMap))
		for _, v := range encryptedBallotsMap{
			ciphertext:= new (Ciphertext)
			err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			K := suite.Point()
			err = K.UnmarshalBinary(ciphertext.K)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			C := suite.Point()
			err = C.UnmarshalBinary(ciphertext.C)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			Ks = append(Ks, K)
			Cs = append(Cs, C)

		}

		var shuffleActor shuffle.Actor
		err = ctx.Injector.Resolve(&shuffleActor)
		if err != nil {
			http.Error(w, "failed to resolve shuffle.Actor: " + err.Error(), http.StatusInternalServerError)
			return
		}

		publicKey := suite.Point()

		err = publicKey.UnmarshalBinary(simpleElection.Pubkey)
		if err != nil {
			http.Error(w, "failed to unmarshal public key: " + err.Error(), http.StatusInternalServerError)
			return
		}

		KsShuffled, CsShuffled, prf, err := shuffleActor.Shuffle(collectiveAuthority, "Ed25519",
			Ks, Cs, publicKey)

		if err != nil {
			http.Error(w, "failed to shuffle: " + err.Error(), http.StatusInternalServerError)
			return
		}

		shuffledBallots := make([][]byte, 0, len(KsShuffled))

		for i := 0; i < len(KsShuffled); i++ {

			kMarshalled, err := KsShuffled[i].MarshalBinary()
			if err != nil {
				http.Error(w, "failed to marshall kyber.Point: " + err.Error(), http.StatusInternalServerError)
				return
			}

			cMarshalled, err := CsShuffled[i].MarshalBinary()
			if err != nil {
				http.Error(w, "failed to marshall kyber.Point: " + err.Error(), http.StatusInternalServerError)
				return
			}

			ciphertext := Ciphertext{K: kMarshalled, C: cMarshalled}
			js, err := json.Marshal(ciphertext)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			shuffledBallots = append(shuffledBallots, js)

		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager := getManager(signer, a.client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		shuffleBallotsTransaction := ShuffleBallotsTransaction{
			ElectionID:      shuffleBallotsRequest.ElectionID,
			UserId:          shuffleBallotsRequest.UserId,
			ShuffledBallots: shuffledBallots,
			Proof:           prf,
		}

		js, err := json.Marshal(shuffleBallotsTransaction)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}


		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			//todo : should be global variable
			Key:   "go.dedis.ch/dela.ContractArg",
			Value: []byte("go.dedis.ch/dela.Evoting"),
		}
		args[1] = txn.Arg{
			//todo : should be global variable
			Key:   "evoting:command",
			Value: []byte("SHUFFLE_BALLOTS"),
		}
		args[2] = txn.Arg{
			//todo : should be global variable
			Key:   "evoting:shuffleBallotsArgs",
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		//var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
					http.Error(w, "Transaction refused : " + msg, http.StatusInternalServerError)
					return
				}

				response := ShuffleBallotsResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				dela.Logger.Info().Msg("ShuffleProblem")
				return
			}
		}

		http.Error(w, "Transaction not found in the block", http.StatusInternalServerError)
		return

	})

	http.HandleFunc(decryptBallotsEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(decryptBallotsEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		decryptBallotsRequest := new (DecryptBallotsRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(decryptBallotsRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		proof, err := service.GetProof([]byte(decryptBallotsRequest.ElectionID))
		if err != nil {
			http.Error(w, "failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		simpleElection := new (types.SimpleElection)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
		if err != nil {
			http.Error(w, "failed to unmarshall SimpleElection: " + err.Error(), http.StatusInternalServerError)
			return
		}

		if simpleElection.Status != types.ShuffledBallots{
			http.Error(w, "The ballots must have been shuffled !", http.StatusUnauthorized)
			return
		}

		if simpleElection.AdminId != decryptBallotsRequest.UserId{
			http.Error(w, "Only the admin can decrypt the ballots !", http.StatusUnauthorized)
			return
		}

		Ks := make([]kyber.Point, 0, len(simpleElection.ShuffledBallots))
		Cs := make([]kyber.Point, 0, len(simpleElection.ShuffledBallots))

		for _, v := range simpleElection.ShuffledBallots{
			ciphertext:= new (Ciphertext)
			err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			K := suite.Point()
			err = K.UnmarshalBinary(ciphertext.K)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			C := suite.Point()
			err = C.UnmarshalBinary(ciphertext.C)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			Ks = append(Ks, K)
			Cs = append(Cs, C)
		}

		// todo : implement a dkg call to decrypt multiple ciphertexts

		var dkgActor dkg.Actor
		err = ctx.Injector.Resolve(&dkgActor)
		if err != nil {
			http.Error(w, "failed to resolve dkg.Actor: " + err.Error(), http.StatusInternalServerError)
			return
		}

		decryptedBallots := make([]types.SimpleBallot, 0, len(simpleElection.ShuffledBallots))

		for i:=0; i < len(Ks); i++ {
			message, err := dkgActor.Decrypt(Ks[i], Cs[i])
			if err != nil {
				http.Error(w, "failed to decrypt (K,C): " + err.Error(), http.StatusInternalServerError)
				return
			}

			decryptedBallots = append(decryptedBallots, types.SimpleBallot{Vote: string(message)})
		}

		var p pool.Pool
		err = ctx.Injector.Resolve(&p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager := getManager(signer, a.client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		decryptBallotsTransaction := DecryptBallotsTransaction{
			ElectionID:       decryptBallotsRequest.ElectionID,
			UserId:           decryptBallotsRequest.UserId,
			DecryptedBallots: decryptedBallots,
		}

		js, err := json.Marshal(decryptBallotsTransaction)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   "go.dedis.ch/dela.ContractArg",
			Value: []byte("go.dedis.ch/dela.Evoting"),
		}
		args[1] = txn.Arg{
			Key:   "evoting:command",
			Value: []byte("DECRYPT_BALLOTS"),
		}
		args[2] = txn.Arg{
			Key:   "evoting:decryptBallotsArgs",
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		//var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
					http.Error(w, "Transaction refused : " + msg, http.StatusInternalServerError)
					return
				}

				response := DecryptBallotsResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		getElectionResultRequest := new (GetElectionResultRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(getElectionResultRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		proof, err := service.GetProof([]byte(getElectionResultRequest.ElectionID))
		if err != nil {
			http.Error(w, "failed to read on the blockchain: " + err.Error(), http.StatusInternalServerError)
			return
		}

		simpleElection := new (types.SimpleElection)
		err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
		if err != nil {
			http.Error(w, "failed to unmarshall SimpleElection: " + err.Error(), http.StatusInternalServerError)
			return
		}

		if simpleElection.Status != types.ResultAvailable{
			http.Error(w, "The result is not available.", http.StatusUnauthorized)
			return
		}

		response := GetElectionResultResponse{Result: simpleElection.DecryptedBallots}

		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return

	})

	http.HandleFunc(cancelElectionEndpoint, func(w http.ResponseWriter, r *http.Request){

		dela.Logger.Info().Msg(cancelElectionEndpoint)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cancelElectionRequest := new (CancelElectionRequest)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(cancelElectionRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		signer, err := getSigner(signerFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		manager := getManager(signer, a.client)

		err = manager.Sync()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		cancelElectionTransaction := CancelElectionTransaction{
			ElectionID: cancelElectionRequest.ElectionID,
			UserId:     cancelElectionRequest.UserId,
		}

		js, err := json.Marshal(cancelElectionTransaction)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		args := make([]txn.Arg, 3)
		args[0] = txn.Arg{
			Key:   "go.dedis.ch/dela.ContractArg",
			Value: []byte("go.dedis.ch/dela.Evoting"),
		}
		args[1] = txn.Arg{
			Key:   "evoting:command",
			Value: []byte("CANCEL_ELECTION"),
		}
		args[2] = txn.Arg{
			Key:   "evoting:cancelElectionArgs",
			Value: js,
		}

		tx, err := manager.Make(args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var service ordering.Service
		err = ctx.Injector.Resolve(&service)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		watchCtx, cancel := context.WithTimeout(context.Background(), createElectionTimeout)
		defer cancel()

		events := service.Watch(watchCtx)

		err = p.Add(tx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
					http.Error(w, "Transaction refused : " + msg, http.StatusInternalServerError)
					return
				}

				response := CancelElectionResponse{
				}

				js, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				_, err = w.Write(js)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
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
		return nil, nil, xerrors.Errorf("failed to decode public key: %v", err)
	}

	return addr, pubkey, nil
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

// createElectionTestAction is an action to
//
// - implements node.ActionTemplate
type createElectionTestAction struct {
}

// Execute implements node.ActionTemplate. It creates
func (a *createElectionTestAction) Execute(ctx node.Context) error {

	createSimpleElectionRequest := CreateSimpleElectionRequest{
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

	castVoteRequest := CastVoteRequest{
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

	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall SimpleElection : %v", err)
	}



	dela.Logger.Info().Msg("Length encrypted ballots : " + strconv.Itoa(len(simpleElection.EncryptedBallots)))
	dela.Logger.Info().Msg("Ballot of user1 : " + string(simpleElection.EncryptedBallots["user1"]))

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

	createSimpleElectionRequest := CreateSimpleElectionRequest{
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

	createSimpleElectionResponse := new(CreateSimpleElectionResponse)

	err = json.NewDecoder(bytes.NewBuffer(body)).Decode(createSimpleElectionResponse)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall CastVoteTransaction : %v", err)
	}

	electionId := createSimpleElectionResponse.ElectionID

	proof, err := service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}


	dela.Logger.Info().Msg("Proof : " + string(proof.GetValue()))
	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + simpleElection.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Admin Id of the election : " + simpleElection.AdminId)
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))

	// ##################################### CREATE SIMPLE ELECTION ####################################################

	// ##################################### GET ELECTION INFO #########################################################

	dela.Logger.Info().Msg("----------------------- GET ELECTION INFO : ")

	getElectionInfoRequest := GetElectionInfoRequest{
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

	simpleElection = new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + simpleElection.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))
	dela.Logger.Info().Msg("Pubkey of the election : " + string(simpleElection.Pubkey))
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

	castVoteRequest := CastVoteRequest{
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

	castVoteRequest = CastVoteRequest{
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

	castVoteRequest = CastVoteRequest{
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

	simpleElection = new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Length encrypted ballots : " + strconv.Itoa(len(simpleElection.EncryptedBallots)))
	dela.Logger.Info().Msg("Ballot of user1 : " + string(simpleElection.EncryptedBallots["user1"]))
	dela.Logger.Info().Msg("Ballot of user2 : " + string(simpleElection.EncryptedBallots["user2"]))
	dela.Logger.Info().Msg("Ballot of user3 : " + string(simpleElection.EncryptedBallots["user3"]))
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))

	// ##################################### CAST BALLOTS ##############################################################

	// ###################################### CLOSE ELECTION ###########################################################

	dela.Logger.Info().Msg("----------------------- CLOSE ELECTION : ")

	closeElectionRequest := CloseElectionRequest{
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

	simpleElection = new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + simpleElection.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Admin Id of the election : " + simpleElection.AdminId)
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))

	// ##################################### CLOSE ELECTION ############################################################

	// ###################################### SHUFFLE BALLOTS ##########################################################

	dela.Logger.Info().Msg("----------------------- SHUFFLE BALLOTS : ")

	roster, err := a.readMembers(ctx)
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	shuffleBallotsRequest := ShuffleBallotsRequest{
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

	proof, err = service.GetProof([]byte(electionId))
	if err != nil {
		return xerrors.Errorf("failed to read on the blockchain: %v", err)
	}

	simpleElection = new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + simpleElection.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))
	dela.Logger.Info().Msg("Number of shuffled ballots : " + strconv.Itoa(len(simpleElection.ShuffledBallots)))
	dela.Logger.Info().Msg("Number of encrypted ballots : " + strconv.Itoa(len(simpleElection.EncryptedBallots)))

	// ###################################### SHUFFLE BALLOTS ##########################################################

	// ###################################### DECRYPT BALLOTS ##########################################################

	dela.Logger.Info().Msg("----------------------- DECRYPT BALLOTS : ")


	decryptBallotsRequest := DecryptBallotsRequest{
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

	simpleElection = new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + simpleElection.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))
	dela.Logger.Info().Msg("Number of decrypted ballots : " + strconv.Itoa(len(simpleElection.DecryptedBallots)))
	dela.Logger.Info().Msg("decrypted ballots [0] : " + simpleElection.DecryptedBallots[0].Vote)
	dela.Logger.Info().Msg("decrypted ballots [1] : " + simpleElection.DecryptedBallots[1].Vote)
	dela.Logger.Info().Msg("decrypted ballots [2] : " + simpleElection.DecryptedBallots[2].Vote)


	// ###################################### DECRYPT BALLOTS ##########################################################

	// ###################################### GET ELECTION RESULT ######################################################

	dela.Logger.Info().Msg("----------------------- GET ELECTION RESULT : ")


	getElectionResultRequest := GetElectionResultRequest{
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

	simpleElection = new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(proof.GetValue())).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	dela.Logger.Info().Msg("Title of the election : " + simpleElection.Title)
	dela.Logger.Info().Msg("ID of the election : " + string(simpleElection.ElectionID))
	dela.Logger.Info().Msg("Status of the election : " + strconv.Itoa(int(simpleElection.Status)))
	dela.Logger.Info().Msg("Number of decrypted ballots : " + strconv.Itoa(len(simpleElection.DecryptedBallots)))
	dela.Logger.Info().Msg("decrypted ballots [0] : " + simpleElection.DecryptedBallots[0].Vote)
	dela.Logger.Info().Msg("decrypted ballots [1] : " + simpleElection.DecryptedBallots[1].Vote)
	dela.Logger.Info().Msg("decrypted ballots [2] : " + simpleElection.DecryptedBallots[2].Vote)


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

	ballot := Ciphertext{K: Kmarshalled, C: Cmarshalled}
	js, err := json.Marshal(ballot)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshall Ciphertext: %v", err)
	}

	return js, nil

}

func (a scenarioTestAction) readMembers(ctx node.Context) ( []CollectiveAuthorityMember, error) {
	members := ctx.Flags.StringSlice("member")

	roster := make([]CollectiveAuthorityMember, len(members))

	for i, member := range members {
		addr, pubkey, err := decodeMemberFromContext(member)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode: %v", err)
		}

		roster[i] = CollectiveAuthorityMember{
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
