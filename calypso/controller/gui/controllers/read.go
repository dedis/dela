package controllers

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"text/template"

	"go.dedis.ch/dela/calypso/controller/gui/models"
	"go.dedis.ch/dela/ledger/arc"
)

// ReadHandler handles the read requests
func (c Ctrl) ReadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			c.readGET(w, r)
		case http.MethodPost:
			c.readPOST(w, r)
		default:
			c.renderHTTPError(w, "only GET and POST requests allowed", http.StatusBadRequest)
		}
	}
}

func (c Ctrl) readGET(w http.ResponseWriter, r *http.Request) {

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/read.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var viewData = struct {
		Title       string
		PostMessage string
	}{
		"Read a message",
		"",
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c Ctrl) readPOST(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	msgIDStr := r.PostForm.Get("msgID")
	if msgIDStr == "" {
		c.renderHTTPError(w, "message ID is empty", http.StatusBadRequest)
		return
	}

	msgIDBuf, err := hex.DecodeString(msgIDStr)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	identity := r.PostForm.Get("identity")
	if identity == "" {
		c.renderHTTPError(w, "C is empty", http.StatusBadRequest)
		return
	}

	foreignID := models.NewIdentity(identity)
	idents := []arc.Identity{foreignID}

	msgBuf, err := c.caly.Read(msgIDBuf, idents...)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/read.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	viewMessage := fmt.Sprintf("Message fetched!\nMessage: %s", msgBuf)

	var viewData = struct {
		Title       string
		PostMessage string
	}{
		"Read a message",
		viewMessage,
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}
