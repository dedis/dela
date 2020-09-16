package access

import "fmt"

// ContractCredentials defines the credentials for a contract. It contains the
// name of the contract and an associated command.
type ContractCredentials struct {
	id       []byte
	contract string
	command  string
}

// NewContractCreds creates new credentials from the associated identifier, the
// name of the contract and its command.
func NewContractCreds(id []byte, contract, command string) ContractCredentials {
	return ContractCredentials{
		id:       id,
		contract: contract,
		command:  command,
	}
}

// GetID returns the identifier for the credentials.
func (cc ContractCredentials) GetID() []byte {
	return append([]byte{}, cc.id...)
}

// GetRule returns the scope of the credentials.
func (cc ContractCredentials) GetRule() string {
	return fmt.Sprintf("%s:%s", cc.contract, cc.command)
}
