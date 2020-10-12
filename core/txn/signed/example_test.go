package signed

import (
	"fmt"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/crypto/bls"
)

func ExampleTransactionManager_Make() {
	signer := bls.NewSigner()

	manager := NewManager(signer, exampleClient{nonce: 5})

	tx, err := manager.Make()
	if err != nil {
		panic("failed to create first transaction: " + err.Error())
	}

	fmt.Println(tx.GetNonce())

	err = manager.Sync()
	if err != nil {
		panic("failed to synchronize: " + err.Error())
	}

	tx, err = manager.Make()
	if err != nil {
		panic("failed to create second transaction: " + err.Error())
	}

	fmt.Println(tx.GetNonce())

	// Output: 0
	// 5
}

// exampleClient is an example of a manager client. It always synchronize the
// manager to the nonce value.
//
// - implements signed.Client
type exampleClient struct {
	nonce uint64
}

// GetNonce implements signed.Client. It always return the same nonce for
// simplicity.
func (cl exampleClient) GetNonce(identity access.Identity) (uint64, error) {
	return cl.nonce, nil
}
