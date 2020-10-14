//
// Documentation Last Review: 07.10.2020
//

package mino

import "go.dedis.ch/dela/serde"

// SimpleResponse is a response that can either return a message, or an error if
// the request has failed.
//
// - implements mino.Response
type simpleResponse struct {
	from Address
	msg  serde.Message
	err  error
}

// NewResponse creates a response that will return the message.
func NewResponse(from Address, msg serde.Message) Response {
	return simpleResponse{
		from: from,
		msg:  msg,
	}
}

// NewResponseWithError creates a response that will return an error instead of
// a message.
func NewResponseWithError(from Address, err error) Response {
	return simpleResponse{
		from: from,
		err:  err,
	}
}

// GetFrom implements mino.Response. It returns the address the response
// originates from.
func (resp simpleResponse) GetFrom() Address {
	return resp.from
}

// GetMessageOrError implements mino.Response. It returns either a message or an
// error.
func (resp simpleResponse) GetMessageOrError() (serde.Message, error) {
	return resp.msg, resp.err
}
