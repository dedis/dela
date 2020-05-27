package controller

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"go.dedis.ch/dela/cmd"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"golang.org/x/xerrors"
)

// CertAction is an action to list the certificates known by the server.
//
// - implements cmd.Action
type certAction struct{}

// Prepare implements cmd.Action. It does nothing.
func (a certAction) Prepare(ctx cmd.Context) ([]byte, error) {
	return nil, nil
}

// Execute implements cmd.Action. It prints the list of certificates known by
// the server with the address associated and the expiration date.
func (a certAction) Execute(req cmd.Request) error {
	var m minogrpc.Joinable

	err := req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	m.GetCertificateStore().Range(func(addr mino.Address, cert *tls.Certificate) bool {
		fmt.Fprintf(req.Out, "Address: %v Certificate: %v\n", addr, cert.Leaf.NotAfter)
		return true
	})

	return nil
}

type tokenRequest struct {
	Expiration time.Duration
}

// TokenAction is an action to generate a token that will be valid for another
// server to join the network of participants.
//
// - implements cmd.Action
type tokenAction struct{}

// Prepare implements cmd.Action. It marshals the token request with the
// expiration time.
func (a tokenAction) Prepare(ctx cmd.Context) ([]byte, error) {
	req := tokenRequest{
		Expiration: ctx.Duration("expiration"),
	}

	buffer, err := json.Marshal(req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return buffer, nil
}

// Execute implements cmd.Action. It generates a token that will be valid for
// the amount of time given in the request.
func (a tokenAction) Execute(req cmd.Request) error {
	dec := json.NewDecoder(req.In)

	input := tokenRequest{}
	err := dec.Decode(&input)
	if err != nil {
		return xerrors.Errorf("couldn't decode input: %v", err)
	}

	var m minogrpc.Joinable
	err = req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	token := m.Token(input.Expiration)

	digest, err := m.GetCertificateStore().Hash(m.GetCertificate())
	if err != nil {
		return xerrors.Errorf("couldn't hash certificate: %v", err)
	}

	fmt.Fprintf(req.Out, "--token %s --cert-hash %s\n",
		token, base64.StdEncoding.EncodeToString(digest))

	return nil
}

type joinRequest struct {
	Token    string
	Addr     string
	CertHash string
}

type joinAction struct{}

func (a joinAction) Prepare(ctx cmd.Context) ([]byte, error) {
	req := joinRequest{
		Token:    ctx.String("token"),
		Addr:     ctx.String("address"),
		CertHash: ctx.String("cert-hash"),
	}

	buffer, err := json.Marshal(req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return buffer, nil
}

func (a joinAction) Execute(req cmd.Request) error {
	dec := json.NewDecoder(req.In)

	var input joinRequest
	err := dec.Decode(&input)
	if err != nil {
		return xerrors.Errorf("couldn't decode input: %v", err)
	}

	var m minogrpc.Joinable
	err = req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	cert, err := base64.StdEncoding.DecodeString(input.CertHash)
	if err != nil {
		return xerrors.Errorf("couldn't decode digest: %v", err)
	}

	err = m.Join(input.Addr, input.Token, cert)
	if err != nil {
		return xerrors.Errorf("couldn't join: %v", err)
	}

	return nil
}
