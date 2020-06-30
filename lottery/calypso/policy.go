package calypso

// NewPolicy creates a new policy
func NewPolicy() *Policy {
	return &Policy{}
}

// Policy ...
//
// implements secret.Policy
type Policy struct {
}

// Match implements secret.Policy
func (p *Policy) Match() error {
	return nil
}
