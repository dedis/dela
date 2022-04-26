# Memcoin

Memcoin is the default CLI to handle Dela nodes. You can find it in
"cli/node/memcoin". Be sure to run "go install" in it and add GOPATH/bin to your
PATH environment.

```sh
# Run three nodes
LLVL=info memcoin --config /tmp/node1 start --listen tcp://127.0.0.1:2001
LLVL=info memcoin --config /tmp/node2 start --listen tcp://127.0.0.1:2002
LLVL=info memcoin --config /tmp/node3 start --listen tcp://127.0.0.1:2003

# Share the certificate
memcoin --config /tmp/node2 minogrpc join \
    --address //127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node3 minogrpc join \
    --address //127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)

# Create a new chain with the three nodes
memcoin --config /tmp/node1 ordering setup\
    --member $(memcoin --config /tmp/node1 ordering export)\
    --member $(memcoin --config /tmp/node2 ordering export)\
    --member $(memcoin --config /tmp/node3 ordering export)

# Create a bls signer to sign transactions. Be sure you have the "crypto" binary
# by running "go install" in cli/crypto.
crypto bls signer new --save private.key
crypto bls signer read --path private.key --format BASE64

# Authorize the signer to handle the access contract on each node
memcoin --config /tmp/node1 access add \
    --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)
memcoin --config /tmp/node2 access add \
    --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)
memcoin --config /tmp/node3 access add \
    --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)

# Update the access contract to allow us to use the value contract. Path to
# private.key is relative to the location where the node has been started.
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Access\
    --args access:grant_id --args 0200000000000000000000000000000000000000000000000000000000000000\
    --args access:grant_contract --args go.dedis.ch/dela.Value\
    --args access:grant_command --args all\
    --args access:identity --args $(crypto bls signer read --path private.key --format BASE64_PUBKEY)\
    --args access:command --args GRANT

# store a value on the value contract
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:key --args "key1"\
    --args value:value --args "value1"\
    --args value:command --args WRITE

# list the values stored on the value contract
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:command --args LIST
```