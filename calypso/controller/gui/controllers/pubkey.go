package controllers

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"text/template"
)

// this file hold the controller for the client execution, as opposed to the
// admin ones.

// PubkeyHandler handles the public key request
func (c Ctrl) PubkeyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			c.pubkeyGET(w, r)
		default:
			c.renderHTTPError(w, "only GET request allowed", http.StatusBadRequest)
		}
	}
}

func (c Ctrl) pubkeyGET(w http.ResponseWriter, r *http.Request) {
	pubkey, err := c.caly.GetPublicKey()
	if err != nil {
		fmt.Println("failed to get pubkey:", err)
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pubkeyHex := hex.EncodeToString(pubkeyBuf)

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/pubkey.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var viewData = struct {
		Title  string
		Pubkey string
	}{
		"Collective public key",
		pubkeyHex,
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
