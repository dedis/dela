package controllers

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"text/template"

	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/util/random"
)

// EncryptHandler handles the encryption
func (c Ctrl) EncryptHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			c.encryptGET(w, r)
		case http.MethodPost:
			c.encryptPOST(w, r)
		default:
			c.renderHTTPError(w, "only GET and POST requests allowed", http.StatusBadRequest)
		}
	}
}

func (c Ctrl) encryptGET(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/encrypt.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var viewData = struct {
		Title       string
		PostMessage string
	}{
		"Encrypt a message",
		"",
	}

	err = t.ExecuteTemplate(w, "layout", viewData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c Ctrl) encryptPOST(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	message := r.PostForm.Get("message")
	if message == "" {
		c.renderHTTPError(w, "message empty", http.StatusBadRequest)
		return
	}

	pubkeyStr := r.PostForm.Get("pubkey")
	if pubkeyStr == "" {
		c.renderHTTPError(w, "pubkey empty", http.StatusBadRequest)
		return
	}

	pubkeyBuf, err := hex.DecodeString(pubkeyStr)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pubkey := dkg.Suite.Point()
	err = pubkey.UnmarshalBinary(pubkeyBuf)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	kPoint, cPoint, remainder, err := encrypt([]byte(message), pubkey)
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(remainder) != 0 {
		c.renderHTTPError(w, fmt.Sprintf("message too long, %d bytes could not "+
			"be embeded", len(remainder)), http.StatusBadRequest)
		return
	}

	kBuf, err := kPoint.MarshalBinary()
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cBuf, err := cPoint.MarshalBinary()
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	khex := hex.EncodeToString(kBuf)
	chex := hex.EncodeToString(cBuf)

	t, err := template.ParseFiles(c.abs("gui/views/layout.gohtml"),
		c.abs("gui/views/encrypt.gohtml"))
	if err != nil {
		c.renderHTTPError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	viewMessage := fmt.Sprintf("Message encrypted!\nPlease save those "+
		"information:\nK: %s\nC: %s", khex, chex)

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

func encrypt(message []byte, pubkey kyber.Point) (
	K kyber.Point, C kyber.Point, remainder []byte, err error) {

	// Embed the message (or as much of it as will fit) into a curve point.
	M := dkg.Suite.Point().Embed(message, random.New())
	max := dkg.Suite.Point().EmbedLen()
	if max > len(message) {
		max = len(message)
	}
	remainder = message[max:]
	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := dkg.Suite.Scalar().Pick(random.New()) // ephemeral private key
	K = dkg.Suite.Point().Mul(k, nil)          // ephemeral DH public key
	S := dkg.Suite.Point().Mul(k, pubkey)      // ephemeral DH shared secret
	C = S.Add(S, M)                            // message blinded with secret

	return K, C, remainder, nil
}
