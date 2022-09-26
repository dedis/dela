# **Mi**nimalistic **N**etwork **O**verlay

Mino is an abstraction of a network overlay which provides a high-level API to
send messages to a list of participants. A distributed system may involve
hundreds of nodes which have to talk to each other, with the worst case being to
talk to all of the participants. In that case, it is inconceivable to open n^2
connections, and this is where the overlay improves the situation.

It provides two approaches: a classic RPC call and a streaming RPC. In the
former, it will contact some nodes and always returns a reply (which can be
empty) so that the sender knows who fails. In the later, the orchestrator (the
initiator of the protocol) opens a stream to one of the participant which will
open to others according to a routing algorithm. The algorithm will then define
the fault-tolerance of the system.

## Players and Addresses

Mino uses an abstraction of the roster that will be implied in a protocol.

```go
type Players interface {

	// Take should a subset of the players according to the filters.
	Take(...FilterUpdater) Players

	// AddressIterator returns an iterator that prevents changes of the
	// underlying array and save memory by iterating over the same array.
	AddressIterator() AddressIterator

	// Len returns the length of the set of players.
	Len() int
}
```

It provides simple primitives to filter and get the list of addresses. Each
implementation of Mino has its own address representation. Minoch uses Go
channels and therefore uses string identifiers, whereas Minogrpc uses actual
network addresses.

This interface can later be extended to add more information to the identity of
a participant, like a public key that will be used for collective signing.

## Namespaces and RPCs

The Mino interface provides two functions to create an endpoint that can be
called by others:

```go
type Mino interface {

    ...

	MakeNamespace(namespace string) (Mino, error)

	MakeRPC(name string, h Handler, f serde.Factory) (RPC, error)
}
```

When a service needs to create an RPC, it will create its own namespace so that
there is no conflict with others, and then create an RPC with a unique name:

```go
m := NewMino()

statusSrvc, err := m.MakeNamespace("status")
if err != nil { ... }

rpc, err := statusSrvc.MakeRPC("health", healthHandler{}, healthFac{})
if err != nil { ... }

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

resps, err := rpc.Call(ctx, HealthRequest{}, roster)
if err != nil { ... }

for resp := range resps {
    ...
}
```

The namespace and rpc combination can be seen as an URI in a web API, like
`go.dedis.ch/status/health` in the example above. You can of course chain the
namespaces as much as you want.

## API

### Call (unicast-based protocol)

Call is one of the API provided by Mino. It takes a context, a message and the
list of participants as input parameters.

The context can be used to cancel the protocol earlier if necessary. When the
context is done, the connection to other peers will be shutdown  and
resources cleaned up.

A normal execution of the call will send the message to all the participants and
the channel of responses will be populated as soon as a reply arrives. The reply
can either contain an actual message, or an error explaining why a participant
could not return the reply. The channel is closed after all the responses are
populated.

### Stream (bidirectional stream-based protocol)

Stream is one of the API provided by Mino. It takes a context and the list of
participants as input parameters:

```go
sender, receiver, err := rpc.Stream(ctx context.Context, players Players)
```

The context defines when the protocol is done, and it should therefore always be
canceled at some point. When it arrives, all the connections are shut down and
the resources are cleaned up.

Unlike a call, the orchestrator of a protocol will contact **one** of the
participants which will be the root for the routing algorithm. It will then
relays the messages according to the algorithm and create relays to other peers
when necessary. For instance, for a tree-based algorithm, it *could* look like:

```
                               Orchestrator
                                     |
                                  __ A __
                                 /       \
                                B         C
                              / | \     /   \
                             D  E  F   G     H
```

A message coming from F would then be relayed through B and A to reach the right
side of the tree. This kind of algorithm is efficient in terms of distributed
load but is very sensible to failures so this is of course only an example.

#### Example

This illustrates how to use the stream API to implement a simple ping service, 
where a message is echoed back to the node who sent it.

```go
func demo() {
    // This mino will be the orchestrator
    m, err := NewMinogrpc(addr, tree.NewRouter(AddressFactory{}))
    if err {...}

    // We provide the handler that each node will execute
    rpc, err := m.MakeRPC("test", handler{}, aMessageFactory)
    if err {...}

    // Players is the list of all the participants
    sender, receiver, err := rpc.Stream(ctx context.Context, players Players)
    if err {...}

    // We send a message to one of the participant
    err := <-sender.Send(aMessage, anAddress)
    if err {...}

    // The participant will receive the message and execute the handler, which will
    // send back the message to us.
    from, msg, err := recv.Recv(context.Background())
}

type handler struct {}

// Stream implements mino.Handler. It simply sends back the message that it 
// receives.
func (h handler) Stream(out mino.Sender, in mino.Receiver) error {
    from, msg, err := in.Recv(context.Background())
    if err != nil {
        return err
    }

    // Here we are just sending back the message, but one could have a more 
    // complex handler that for example sends messages to other nodes, waits for
    // their replies and does some processing.
    err = <-out.Send(msg, from)
    if err != nil {
        return err
    }

    return nil
}
```

#### Notes

Set these env. flags to show GRPC traces:
```
GRPC_GO_LOG_SEVERITY_LEVEL=info;
GRPC_GO_LOG_VERBOSITY_LEVEL=10;
```

