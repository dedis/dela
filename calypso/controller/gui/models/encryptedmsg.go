package models

import "go.dedis.ch/kyber/v3"

// NewEncrpytedMsg creates a new encrypted message
func NewEncrpytedMsg(k, c kyber.Point) *EncryptedMsg {
	return &EncryptedMsg{
		K: k,
		C: c,
	}
}

// EncryptedMsg represents an encrypted message
//
// - implements calypso.EncryptedMessage
type EncryptedMsg struct {
	K kyber.Point
	C kyber.Point
}

// GetK implements calypso.EncryptedMessage
func (f EncryptedMsg) GetK() kyber.Point {
	return f.K
}

// GetC implements calypso.EncryptedMessage
func (f EncryptedMsg) GetC() kyber.Point {
	return f.C
}
