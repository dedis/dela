package certs_test

import (
	"crypto/x509"
	"fmt"

	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router/tree"
)

func ExampleStorage_Fetch() {
	router := tree.NewRouter(minogrpc.NewAddressFactory())

	m, err := minogrpc.NewMinogrpc(minogrpc.ParseAddress("127.0.0.1", 0), nil, router)
	if err != nil {
		panic("couldn't start mino: " + err.Error())
	}

	store := certs.NewInMemoryStore()

	digest, err := store.Hash(m.GetCertificateChain())
	if err != nil {
		panic("certificate digest failed: " + err.Error())
	}

	err = store.Fetch(m.GetAddress().(session.Address), digest)
	if err != nil {
		panic("fetch failed: " + err.Error())
	}

	certBuf, err := store.Load(m.GetAddress())
	if err != nil {
		panic("while loading certificate: " + err.Error())
	}

	cert, err := x509.ParseCertificate(certBuf)
	if err != nil {
		panic("while parsing certificate")
	}

	fmt.Println("Certificate host", cert.IPAddresses)

	// Output: Certificate host [127.0.0.1]
}
