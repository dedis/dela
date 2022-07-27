// This file contains the implementation of the controller actions.
//
// Documentation Last Review: 07.10.2020
//

package controller

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"golang.org/x/xerrors"
)

// CertAction is an action to list the certificates known by the server.
//
// - implements node.ActionTemplate
type certAction struct{}

// Execute implements node.ActionTemplate. It prints the list of certificates
// known by the server with the address associated and the expiration date.
func (a certAction) Execute(req node.Context) error {
	var m minogrpc.Joinable

	err := req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	m.GetCertificateStore().Range(func(addr mino.Address, chain certs.CertChain) bool {
		buff, _ := addr.MarshalText()
		addrB64 := base64.StdEncoding.EncodeToString(buff)

		certs, err := x509.ParseCertificates(chain)
		if err != nil {
			return false
		}

		certStr := make([]string, len(certs))
		for i, c := range certs {
			certStr[i] = hex.EncodeToString(c.Raw[:8]) + "..."
		}

		fmt.Fprintf(req.Out, "Address: %v (%s) Certificate: %s\n", addr, addrB64, strings.Join(certStr, "<-"))
		return true
	})

	return nil
}

// RemoveAction is an action to remove certificates associated to an address
// from the server.
//
// - implements node.ActionTemplate
type removeAction struct{}

// Execute implements node.ActionTemplate. It removes a certificate based on an
// address.
func (a removeAction) Execute(req node.Context) error {
	var m minogrpc.Joinable

	err := req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	addrBuf, err := base64.StdEncoding.DecodeString(req.Flags.String("address"))
	if err != nil {
		return xerrors.Errorf("failed to decode base64 address: %v", err)
	}

	addr := m.GetAddressFactory().FromText(addrBuf)

	err = m.GetCertificateStore().Delete(addr)
	if err != nil {
		return xerrors.Errorf("failed to delete: %v", err)
	}

	fmt.Fprintf(req.Out, "certificate(s) with address %q removed", addrBuf)

	return nil
}

// TokenAction is an action to generate a token that will be valid for another
// server to join the network of participants.
//
// - implements node.ActionTemplate
type tokenAction struct{}

// Execute implements node.ActionTemplate. It generates a token that will be
// valid for the amount of time given in the request.
func (a tokenAction) Execute(req node.Context) error {
	exp := req.Flags.Duration("expiration")

	var m minogrpc.Joinable
	err := req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	token := m.GenerateToken(exp)

	chain := m.GetCertificateChain()

	digest, err := m.GetCertificateStore().Hash(chain)
	if err != nil {
		return xerrors.Errorf("couldn't hash certificate: %v", err)
	}

	fmt.Fprintf(req.Out, "--token %s --cert-hash %s\n",
		token, base64.StdEncoding.EncodeToString(digest))

	return nil
}

// JoinAction is an action to join a network of participants by providing a
// valid token and the certificate hash.
//
// - implements node.ActionTemplate
type joinAction struct{}

// Execute implements node.ActionTemplate. It parses the request and send the
// join request to the distant node.
func (a joinAction) Execute(req node.Context) error {
	token := req.Flags.String("token")
	certHash := req.Flags.String("cert-hash")

	addrURL, err := url.Parse(req.Flags.String("address"))
	if err != nil {
		return xerrors.Errorf("failed to parse addr: %v", err)
	}

	var m minogrpc.Joinable
	err = req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	cert, err := base64.StdEncoding.DecodeString(certHash)
	if err != nil {
		return xerrors.Errorf("couldn't decode digest: %v", err)
	}

	err = m.Join(addrURL, token, cert)
	if err != nil {
		return xerrors.Errorf("couldn't join: %v", err)
	}

	return nil
}
