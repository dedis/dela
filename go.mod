module go.dedis.ch/dela

go 1.14

require (
	github.com/golang/protobuf v1.3.5
	github.com/opentracing/opentracing-go v1.2.0
	github.com/rs/xid v1.2.1
	github.com/rs/zerolog v1.19.0
	github.com/stretchr/testify v1.6.1
	github.com/urfave/cli/v2 v2.2.0
	go.dedis.ch/dela-apps v0.0.0-20200716135110-fa13414b237a
	go.dedis.ch/kyber/v3 v3.0.13
	go.etcd.io/bbolt v1.3.5
	golang.org/x/net v0.0.0-20201021035429-f5854403a974
	golang.org/x/tools v0.1.0
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/grpc v1.31.1
	gopkg.in/yaml.v2 v2.2.2
)

replace go.dedis.ch/dela-apps => /Users/nkocher/GitHub/dela-apps
