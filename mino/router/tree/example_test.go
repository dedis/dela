package tree

import (
	"fmt"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/session"
)

func ExampleRouter_New() {
	router := NewRouter(session.AddressFactory{})

	addrA := session.NewAddress("127.0.0.1:2000")
	addrB := session.NewAddress("127.0.0.1:3000")

	players := mino.NewAddresses(addrA, addrB)

	table, err := router.New(players, addrA)
	if err != nil {
		panic("routing table failed: " + err.Error())
	}

	routes, voids := table.Forward(table.Make(addrA, []mino.Address{addrB}, []byte{}))
	fmt.Println(voids)

	for to := range routes {
		fmt.Println(to)
	}

	// Output: map[]
	// 127.0.0.1:3000
}

func ExampleTable_PrepareHandshakeFor() {
	routerA := NewRouter(session.AddressFactory{})

	addrA := session.NewAddress("127.0.0.1:2000")
	addrB := session.NewAddress("127.0.0.1:3000")

	players := mino.NewAddresses(addrA, addrB)

	table, err := routerA.New(players, addrA)
	if err != nil {
		panic("routing table failed: " + err.Error())
	}

	handshake := table.PrepareHandshakeFor(addrB)

	// Send the handshake to the address B..

	routerB := NewRouter(session.AddressFactory{})

	tableB, err := routerB.GenerateTableFrom(handshake)
	if err != nil {
		panic("malformed handshake: " + err.Error())
	}

	packet := tableB.Make(addrB, []mino.Address{addrA}, []byte{})

	fmt.Println(packet.GetSource())
	fmt.Println(packet.GetDestination())

	// Output: 127.0.0.1:3000
	// [127.0.0.1:2000]
}
