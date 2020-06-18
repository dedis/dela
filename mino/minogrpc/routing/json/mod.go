package json

type Address []byte

type TreeRouting struct {
	Root      []byte
	Addresses []Address
}
