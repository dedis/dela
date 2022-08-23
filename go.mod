module go.dedis.ch/dela

go 1.14

require (
	github.com/HdrHistogram/hdrhistogram-go v1.0.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/golang/protobuf v1.5.2
	github.com/opentracing-contrib/go-grpc v0.0.0-20200813121455-4a6760c71486
	github.com/opentracing/opentracing-go v1.2.0
	github.com/prometheus/client_golang v1.12.1
	github.com/rs/xid v1.2.1
	github.com/rs/zerolog v1.19.0
	github.com/stretchr/testify v1.7.0
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.4.0+incompatible // indirect
	github.com/urfave/cli/v2 v2.2.0
	go.dedis.ch/kyber/v3 v3.0.13
	go.etcd.io/bbolt v1.3.5
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/net v0.0.0-20211015210444-4f30a5c0130f
	golang.org/x/tools v0.1.11-0.20220316014157-77aa08bb151a
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/grpc v1.45.0
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b // indirect
	gopkg.in/yaml.v2 v2.4.0
)

replace go.dedis.ch/kyber/v3 => /home/mahsa/kyber/kyber