package permissions

// Identity is an abstraction to uniquely identify a signer.
type Identity interface {
}

// AccessControl is an abstraction to verify if an identity has access to a
// specific rule.
type AccessControl interface {
	Match(identity Identity, rule string) bool
}

// AccessControlRegistry is an abstraction of a registry of access controls. It
// is the place where the different rights are stored and it is up to the
// implementation to decide on the medium.
type AccessControlRegistry interface {
	Get(id []byte) (AccessControl, error)
}
