package controller

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/mino"
)

// internalCA is the internal Collective Authority built from the CLI flags
//
// - implements crypto.CollectiveAuthority
type internalCA struct {
	players mino.Players
	pubkeys []ed25519.PublicKey
}

// Len implements mino.Players
func (ca internalCA) Len() int {
	return ca.players.Len()
}

// Take implements mino.Players
func (ca internalCA) Take(fs ...mino.FilterUpdater) mino.Players {
	return ca.players.Take(fs...)
}

// AddressIterator implements mino.AddressIterator
func (ca internalCA) AddressIterator() mino.AddressIterator {
	return ca.players.AddressIterator()
}

func (ca internalCA) GetPublicKey(addr mino.Address) (crypto.PublicKey, int) {
	dela.Logger.Fatal().Msg("GetPublicKey not implemented")
	return nil, 0
}

// PublicKeyIterator implements crypto.CollectiveAuthority
func (ca internalCA) PublicKeyIterator() crypto.PublicKeyIterator {
	return &pkIterator{
		pubkeys: ca.pubkeys,
	}
}

type pkIterator struct {
	pubkeys []ed25519.PublicKey
	index   int
}

func (it pkIterator) Seek(i int) {
	it.index = i
}

func (it pkIterator) HasNext() bool {
	return it.index < len(it.pubkeys)
}

func (it *pkIterator) GetNext() crypto.PublicKey {
	res := it.pubkeys[it.index]
	it.index++
	return res
}
