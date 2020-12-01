// This file contains the implementation of the controller actions.
//
// Documentation Last Review: 07.10.2020
//

package controller

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"golang.org/x/xerrors"
	"os"
	"os/signal"
	"syscall"
	"time"
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

	m.GetCertificateStore().Range(func(addr mino.Address, cert *tls.Certificate) bool {
		fmt.Fprintf(req.Out, "Address: %v Certificate: %v\n", addr, cert.Leaf.NotAfter)
		return true
	})

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

	digest, err := m.GetCertificateStore().Hash(m.GetCertificate())
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
	addr := req.Flags.String("address")
	certHash := req.Flags.String("cert-hash")

	var m minogrpc.Joinable
	err := req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	cert, err := base64.StdEncoding.DecodeString(certHash)
	if err != nil {
		return xerrors.Errorf("couldn't decode digest: %v", err)
	}

	err = m.Join(addr, token, cert)
	if err != nil {
		return xerrors.Errorf("couldn't join: %v", err)
	}

	return nil
}

type streamAction struct{}

func (s streamAction) Execute(req node.Context) error {
	addrs := req.Flags.StringSlice("addresses")
	dela.Logger.Info().Msgf("addrs: %v", addrs)

	var o *minogrpc.Minogrpc
	err := req.Injector.Resolve(&o)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	addresses := make([]mino.Address, len(addrs)+1)
	addresses[0] = o.GetAddress()
	addressFactory := minogrpc.NewAddressFactory()
	for i := 0; i < len(addrs); i++ {
		addresses[i+1] = addressFactory.FromText([]byte(addrs[i]))
		dela.Logger.Info().Msgf("addresses[%d]=%s", i+1, addresses[i+1].String())
	}
	var rpc mino.RPC
	err = req.Injector.Resolve(&rpc)
	if err != nil {
		return xerrors.Errorf("couldn't resolve: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender, receiver, err := rpc.Stream(ctx, mino.NewAddresses(addresses...))
	if err != nil {
		return xerrors.Errorf("error creating stream: %v", err)
	}

	msg := req.Flags.String("message")
	err = <-sender.Send(exampleMessage{value: msg}, addresses[1:]...)
	if err != nil {
		return xerrors.Errorf("error sending message: %v", err)
	}

	quit := make(chan struct{})
	done := func(quit chan struct{}) chan struct{} {
		done := make(chan struct{})
		go func() {
			tick := time.Tick(3 * time.Second)
			for {
				select {
				case <-done:
					dela.Logger.Info().Msg("closing receiver")
					quit <- struct{}{}
					return
				case <-tick:
					ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
					dela.Logger.Info().Msg("Receiving messages from stream...")
					from, reply, err := receiver.Recv(ctx)
					if err != nil {
						dela.Logger.Error().Msgf("error receiving message: %v", err)
						continue
					}
					dela.Logger.Info().Msgf("`%s` says `%s`", from, reply)
				}
			}
		}()
		return done
	}(quit)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan
	dela.Logger.Info().Msg("received ctrl+c")
	done <- struct{}{}
	dela.Logger.Info().Msg("sent kill to goroutine")
	<-quit
	dela.Logger.Info().Msg("killed goroutine")
	return nil
}
