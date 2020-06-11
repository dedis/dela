// Package tmp will be removed after the migration.
package tmp

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/wrappers"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

var serializer = json.NewSerializer()

func ProtoOf(m serde.Message) proto.Message {
	data, err := serializer.Serialize(m)
	if err != nil {
		panic(err)
	}

	pb := &wrappers.BytesValue{
		Value: data,
	}

	return pb
}

func FromProto(pb proto.Message, f serde.Factory) serde.Message {
	data := pb.(*wrappers.BytesValue).Value

	var m serde.Message
	err := serializer.Deserialize(data, f, &m)
	if err != nil {
		panic(err)
	}

	return m
}

func MarshalProto(m proto.Message) []byte {
	container, err := ptypes.MarshalAny(m)
	if err != nil {
		panic(err)
	}

	data, err := proto.Marshal(container)
	if err != nil {
		panic(err)
	}

	return data
}

func UnmarshalProto(data []byte) proto.Message {
	var container any.Any
	err := proto.Unmarshal(data, &container)
	if err != nil {
		panic(err)
	}

	var m ptypes.DynamicAny
	err = ptypes.UnmarshalAny(&container, &m)
	if err != nil {
		panic(err)
	}

	return m.Message
}
