package contract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	internal "go.dedis.ch/dela/internal/testing"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Instance{},
		&SpawnTaskProto{},
		&InvokeTaskProto{},
		&DeleteTaskProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}
