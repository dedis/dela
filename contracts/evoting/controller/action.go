package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/satori/go.uuid"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

const url = "http://localhost:"

const loginEndPoint = "/evoting/login"
const createElectionEndPoint = "/evoting/create"
const createElectionPollingEndPoint = "/evoting/create/polling"
const castVoteEndpoint = "/evoting/cast"
const closeElectionEndpoint = "/evoting/close"
const getElectionResultEndpoint = "/evoting/result"
const getElectionStatusEndpoint = "/evoting/status"
const getElectionInfoEndpoint = "/evoting/info"
const cancelElectionEndpoint = "/evoting/decrypt"

const token = "token"
const signerFilePath = "private.key"

const createElectionTimeout = 2 * time.Second

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

	ElectionIdNonce uint16
	client *client
}

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
	ElectionID uint16
	Title string
	AdminId string
	Candidates []string
	PublicKey []byte
}


type CreateSimpleElectionResponse struct {
	ElectionID uint16
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

		electionId := a.ElectionIdNonce

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

				a.ElectionIdNonce++

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

	http.HandleFunc(castVoteEndpoint, func(w http.ResponseWriter, r *http.Request){

	})

	http.HandleFunc(closeElectionEndpoint, func(w http.ResponseWriter, r *http.Request){

	})

	http.HandleFunc(getElectionResultEndpoint, func(w http.ResponseWriter, r *http.Request){

	})

	http.HandleFunc(getElectionStatusEndpoint, func(w http.ResponseWriter, r *http.Request){

	})

	http.HandleFunc(getElectionInfoEndpoint, func(w http.ResponseWriter, r *http.Request){

	})

	http.HandleFunc(cancelElectionEndpoint, func(w http.ResponseWriter, r *http.Request){

	})

	log.Fatal(http.ListenAndServe(":" + portNumber, nil))

	return nil
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
