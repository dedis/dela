package serde

import "golang.org/x/xerrors"

const (
	notImplementedErr = "not implemented"
)

// UnimplementedMessage is a default implementation of the message interface
// which returns error to each function. It should be embedded to any message
// implementation.
//
// - implement serde.Message
type UnimplementedMessage struct{}

// VisitJSON implements serde.Message. It returns an error.
func (m UnimplementedMessage) VisitJSON() (interface{}, error) {
	return nil, xerrors.New(notImplementedErr)
}

// VisitGob implements serde.Message. It returns an error.
func (m UnimplementedMessage) VisitGob() (interface{}, error) {
	return nil, xerrors.New(notImplementedErr)
}

// VisitProto implements serde.Message. It returns an error.
func (m UnimplementedMessage) VisitProto() (interface{}, error) {
	return nil, xerrors.New(notImplementedErr)
}

// UnimplementedFactory is a default implementation of the factory interface
// where each function returns an error. It should be embedded to any factory
// implementation.
//
// - implement serde.Factory
type UnimplementedFactory struct{}

// VisitJSON implements serde.Factory. It returns an error.
func (m UnimplementedFactory) VisitJSON(FactoryInput) (Message, error) {
	return nil, xerrors.New(notImplementedErr)
}

// VisitGob implements serde.Factory. It returns an error.
func (m UnimplementedFactory) VisitGob(FactoryInput) (Message, error) {
	return nil, xerrors.New(notImplementedErr)
}

// VisitProto implements serde.Factory. It returns an error.
func (m UnimplementedFactory) VisitProto(FactoryInput) (Message, error) {
	return nil, xerrors.New(notImplementedErr)
}
