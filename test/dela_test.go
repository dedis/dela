package integration

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/mino"
)

// dela defines the common interface for a Dela node.
type dela interface {
	Setup(...dela)
	GetMino() mino.Mino
	GetOrdering() ordering.Service
	GetTxManager() txn.Manager
	GetAccessService() access.Service
}
