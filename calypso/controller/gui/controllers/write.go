package controllers

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"text/template"

	"go.dedis.ch/dela/calypso"
	"go.dedis.ch/dela/calypso/controller/gui/models"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/ledger/arc/darc"
)

// WriteHandler handles the write requests
func (c Ctrl) WriteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			c.writeGET(w, r)
		case http.MethodPost:
			c.writePOST(w, r)
		default:
			c.renderHTTPError(w, "only GET and POST requests allowed", http.StatusBadRequest)
		}
	}
}

func (c Ctrl) writeGET(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/write.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var viewData = struct {
		Title       string
		PostMessage string
	}{
		"Write a message",
		"",
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c Ctrl) writePOST(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	kStr := r.PostForm.Get("k")
	if kStr == "" {
		c.renderHTTPError(w, "K is empty", http.StatusBadRequest)
		return
	}

	cStr := r.PostForm.Get("c")
	if cStr == "" {
		c.renderHTTPError(w, "C is empty", http.StatusBadRequest)
		return
	}

	kBuf, err := hex.DecodeString(kStr)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cBuf, err := hex.DecodeString(cStr)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	kPoint := dkg.Suite.Point()
	err = kPoint.UnmarshalBinary(kBuf)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cPoint := dkg.Suite.Point()
	err = cPoint.UnmarshalBinary(cBuf)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	adminIdentity := r.PostForm.Get("adminID")
	if adminIdentity == "" {
		c.renderHTTPError(w, "Admin identity is empty", http.StatusBadRequest)
		return
	}

	readIdentities := r.PostForm["readID"]

	ownerID := models.NewIdentity(adminIdentity)
	d := darc.NewAccess()
	d, err = d.Evolve(calypso.ArcRuleUpdate, ownerID)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	d, err = d.Evolve(calypso.ArcRuleRead, ownerID)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, rid := range readIdentities {
		d, err = d.Evolve(calypso.ArcRuleRead, models.NewIdentity(rid))
		if err != nil {
			c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	id, err := c.caly.Write(models.NewEncrpytedMsg(kPoint, cPoint), d)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	idHex := hex.EncodeToString(id)

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/write.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	viewMessage := fmt.Sprintf("Message saved! Please save the ID:\nID: %s", idHex)

	var viewData = struct {
		Title       string
		PostMessage string
	}{
		"Encrypt a message",
		viewMessage,
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}
