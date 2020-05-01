package contract

import (
	"testing"

	"github.com/golang/protobuf/proto"
	internal "go.dedis.ch/fabric/internal/testing"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Instance{},
		&SpawnActionProto{},
		&InvokeActionProto{},
		&DeleteActionProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}
