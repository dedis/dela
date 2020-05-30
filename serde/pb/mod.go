package pb

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	protov1 "github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

//go:generate protoc -I ./ --go_out=./ ./wrapper.proto

type pbEncoder struct {
	registry *protoregistry.Types
}

func newEncoder() pbEncoder {
	return pbEncoder{
		registry: new(protoregistry.Types),
	}
}

func (e pbEncoder) Encode(m interface{}) ([]byte, error) {
	pb, ok := m.(proto.Message)
	if ok {
		return proto.Marshal(pb)
	}

	pb, err := e.getOrSet(m)
	if err != nil {
		return nil, err
	}

	value := reflect.ValueOf(m)

	fields := pb.ProtoReflect().Type().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		val := value.FieldByName(string(field.Name())).Interface()

		pb.ProtoReflect().Set(field, protoreflect.ValueOf(val))
	}

	return proto.Marshal(pb)
}

func (e pbEncoder) Decode(buffer []byte, m interface{}) error {
	pb, ok := m.(proto.Message)
	if ok {
		return proto.Unmarshal(buffer, pb)
	}

	pb, err := e.getOrSet(m)
	if err != nil {
		return err
	}

	err = proto.Unmarshal(buffer, pb)
	if err != nil {
		return err
	}

	value := reflect.ValueOf(m).Elem()
	fields := pb.ProtoReflect().Type().Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)

		val := pb.ProtoReflect().Get(field).Interface()

		value.FieldByName(string(field.Name())).Set(reflect.ValueOf(val))
	}

	return nil
}

func (e pbEncoder) Wrap(m interface{}) ([]byte, error) {
	value, err := e.Encode(m)
	if err != nil {
		return nil, err
	}

	pb := &Wrapper{
		Type:  serde.KeyOf(m),
		Value: value,
	}

	buffer, err := proto.Marshal(protov1.MessageV2(pb))
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

func (e pbEncoder) Unwrap(buffer []byte) (interface{}, error) {
	pb := &Wrapper{}
	err := proto.Unmarshal(buffer, protov1.MessageV2(pb))
	if err != nil {
		return nil, err
	}

	m, ok := serde.New(pb.Type)
	if !ok {
		return nil, xerrors.Errorf("unknown message <%s>", pb.Type)
	}

	err = e.Decode(pb.Value, m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (e pbEncoder) getOrSet(m interface{}) (proto.Message, error) {
	typ := reflect.TypeOf(m)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	desc, err := e.registry.FindMessageByName(type2name(typ))
	if err != nil {
		// Description is missing so let's fill it.
		desc, err = e.setMsgDescription(typ)
		if err != nil {
			return nil, err
		}
	}

	return dynamicpb.NewMessage(desc.Descriptor()), nil
}

func (e pbEncoder) setMsgDescription(typ reflect.Type) (protoreflect.MessageType, error) {
	msgDesc := &descriptorpb.DescriptorProto{
		Name:  str2ptr(typ.Name()),
		Field: make([]*descriptorpb.FieldDescriptorProto, 0, typ.NumField()),
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		msgDesc.Field = append(msgDesc.Field, &descriptorpb.FieldDescriptorProto{
			Name:   str2ptr(field.Name),
			Number: int2ptr(int32(i + 1)),
			Type:   fieldType(field),
		})
	}

	fileDesc := &descriptorpb.FileDescriptorProto{
		Name:        str2ptr("messages.proto"),
		Package:     str2ptr(pkgName(typ)),
		MessageType: []*descriptorpb.DescriptorProto{msgDesc},
		Syntax:      str2ptr("proto3"),
	}

	file, err := protodesc.NewFile(fileDesc, nil)
	if err != nil {
		return nil, err
	}

	msg := dynamicpb.NewMessageType(file.Messages().Get(0))

	err = e.registry.RegisterMessage(msg)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func type2name(typ reflect.Type) protoreflect.FullName {
	return protoreflect.FullName(fmt.Sprintf("%s.%s", pkgName(typ), typ.Name()))
}

func pkgName(typ reflect.Type) string {
	url, err := url.Parse("//" + typ.PkgPath())
	if err != nil {
		panic(err)
	}

	return strings.ReplaceAll(url.RequestURI()[1:], "/", ".")
}

func str2ptr(v string) *string {
	return &v
}

func int2ptr(v int32) *int32 {
	return &v
}

func fieldType(typ reflect.StructField) *descriptorpb.FieldDescriptorProto_Type {
	var fieldType descriptorpb.FieldDescriptorProto_Type

	switch typ.Type.Kind() {
	case reflect.String:
		fieldType = descriptorpb.FieldDescriptorProto_TYPE_STRING
	default:
		panic("nope")
	}

	return &fieldType
}
