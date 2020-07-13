package controller

import (
	"encoding/hex"
	"encoding/json"
	"net/http"

	"go.dedis.ch/dela/calypso"
)

// this file hold the controller for the client execution, as opposed to the
// admin ones.

func getPublicKeyHandler(caly *calypso.Caly) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			pubkey, err := caly.GetPublicKey()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			pubkeyBuf, err := pubkey.MarshalBinary()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			pubkeyHex := hex.EncodeToString(pubkeyBuf)

			var resp = struct {
				Pubkey string
			}{
				pubkeyHex,
			}

			respJSON, err := json.MarshalIndent(resp, "", "")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(respJSON)
		default:
			http.Error(w, "only GET request allowed", http.StatusBadRequest)
		}
	}
}
