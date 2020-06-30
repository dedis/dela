package calypso

// NewApproval return a new Approval
func NewApproval() *Approval {
	return &Approval{}
}

// Approval ...
//
// implements lottery.Approval
type Approval struct {
}

// GetIdentities implements lottery.Approval
func (a *Approval) GetIdentities() [][]byte {
	return nil
}
