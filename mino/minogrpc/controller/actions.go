package controller

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"golang.org/x/xerrors"
)

// CertAction is an action to list the certificates known by the server.
//
// - implements node.ActionTemplate
type certAction struct{}

// Prepare implements node.ActionTemplate. It does nothing.
func (a certAction) GenerateRequest(ctx cli.Flags) ([]byte, error) {
	return nil, nil
}

// Execute implements node.ActionTemplate. It prints the list of certificates
// known by the server with the address associated and the expiration date.
func (a certAction) Execute(req node.Context) error {
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
// - implements node.ActionTemplate
type tokenAction struct{}

// Prepare implements node.ActionTemplate. It marshals the token request with
// the expiration time.
func (a tokenAction) GenerateRequest(ctx cli.Flags) ([]byte, error) {
	req := tokenRequest{
		Expiration: ctx.Duration("expiration"),
	}

	buffer, err := json.Marshal(req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return buffer, nil
}

// Execute implements node.ActionTemplate. It generates a token that will be
// valid for the amount of time given in the request.
func (a tokenAction) Execute(req node.Context) error {
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

	token := m.GenerateToken(input.Expiration)

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

// JoinAction is an action to join a network of participants by providing a
// valid token and the certificate hash.
//
// - implements node.ActionTemplate
type joinAction struct{}

// Prepare implements node.ActionTemplate. It returns the join request
// containing the token, the address and the certificate hash.
func (a joinAction) GenerateRequest(ctx cli.Flags) ([]byte, error) {
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

// Execute implements node.ActionTemplate. It parses the request and send the
// join request to the distant node.
func (a joinAction) Execute(req node.Context) error {
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
