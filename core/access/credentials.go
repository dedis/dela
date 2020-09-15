package access

import "fmt"

type ContractCredentials struct {
	id       []byte
	contract string
	command  string
}

func NewContractCreds(id []byte, contract, command string) ContractCredentials {
	return ContractCredentials{
		id:       id,
		contract: contract,
		command:  command,
	}
}

func (cc ContractCredentials) GetID() []byte {
	return append([]byte{}, cc.id...)
}

func (cc ContractCredentials) GetRule() string {
	return fmt.Sprintf("%s:%s", cc.contract, cc.command)
}
