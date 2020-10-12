package certs_test

import (
	"fmt"

	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router/tree"
)

func ExampleStorage_Fetch() {
	router := tree.NewRouter(minogrpc.NewAddressFactory())

	m, err := minogrpc.NewMinogrpc(minogrpc.ParseAddress("127.0.0.1", 0), router)
	if err != nil {
		panic("couldn't start mino: " + err.Error())
	}

	store := certs.NewInMemoryStore()

	digest, err := store.Hash(m.GetCertificate())
	if err != nil {
		panic("certificate digest failed: " + err.Error())
	}

	err = store.Fetch(m.GetAddress().(session.Address), digest)
	if err != nil {
		panic("fetch failed: " + err.Error())
	}

	cert, err := store.Load(m.GetAddress())
	if err != nil {
		panic("while loading certificate: " + err.Error())
	}

	fmt.Println("Certificate host", cert.Leaf.IPAddresses)

	// Output: Certificate host [127.0.0.1]
}
