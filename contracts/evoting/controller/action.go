package controller

import (
	"encoding/json"
	"github.com/satori/go.uuid"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"log"
	"net/http"
)

const loginEndPoint = "/evoting/login"
const createElectionEndPoint = "/evoting/create"
const castVoteEndpoint = "/evoting/cast"
const closeElectionEndpoint = "/evoting/close"
const getElectionResultEndpoint = "/evoting/result"
const getElectionStatusEndpoint = "/evoting/status"
const getElectionInfoEndpoint = "/evoting/info"
const cancelElectionEndpoint = "/evoting/decrypt"

const token = "token"

// TODO : Merge evoting and DKG web server ?

// initHttpServer is an action to initialize the shuffle protocol
//
// - implements node.ActionTemplate
type initHttpServer struct {
}

type LoginResponse struct {
	UserID string
	Token string
}

// Execute implements node.ActionTemplate. It implements the handling of endpoints
// and start the HTTP server
func (a *initHttpServer) Execute(ctx node.Context) error {
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
