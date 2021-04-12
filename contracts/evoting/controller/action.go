package controller

import (
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/dkg"
	"golang.org/x/xerrors"
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


// initHttpServer is an action to initialize the shuffle protocol
//
// - implements node.ActionTemplate
type initHttpServer struct {
}

// TODO : Merge evoting and DKG web server ?

// Execute implements node.ActionTemplate. It implements the handling of endpoints
// and start the HTTP server
func (a *initHttpServer) Execute(ctx node.Context) error {
	portNumber := ctx.Flags.String("portNumber")

	var actor dkg.Actor
	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	http.HandleFunc(loginEndPoint, func(w http.ResponseWriter, r *http.Request){

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
