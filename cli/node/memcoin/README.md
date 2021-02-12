# Use of the value contract

```sh
# Run two nodes
go install && LLVL=info memcoin --config /tmp/node1 start --port 2001
go install && LLVL=info memcoin --config /tmp/node2 start --port 2002
go install && LLVL=info memcoin --config /tmp/node3 start --port 2003
go install && LLVL=info memcoin --config /tmp/node4 start --port 2004
go install && LLVL=info memcoin --config /tmp/node5 start --port 2005
go install && LLVL=info memcoin --config /tmp/node6 start --port 2006
go install && LLVL=info memcoin --config /tmp/node6 start --port 2007
go install && LLVL=info memcoin --config /tmp/node6 start --port 2008
go install && LLVL=info memcoin --config /tmp/node6 start --port 2009
go install && LLVL=info memcoin --config /tmp/node6 start --port 2010


# Share the certificate
memcoin --config /tmp/node2 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node3 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node4 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node5 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node6 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node7 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node8 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node9 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)
memcoin --config /tmp/node10 minogrpc join --address 127.0.0.1:2001 $(memcoin --config /tmp/node1 minogrpc token)

# Create a new chain with the three nodes
memcoin --config /tmp/node1 ordering setup\
    --member $(memcoin --config /tmp/node1 ordering export)\
    --member $(memcoin --config /tmp/node2 ordering export)\
    --member $(memcoin --config /tmp/node3 ordering export)\
    --member $(memcoin --config /tmp/node4 ordering export)\
    --member $(memcoin --config /tmp/node5 ordering export)\
    --member $(memcoin --config /tmp/node6 ordering export)\
    --member $(memcoin --config /tmp/node7 ordering export)\
    --member $(memcoin --config /tmp/node8 ordering export)\
    --member $(memcoin --config /tmp/node9 ordering export)\
    --member $(memcoin --config /tmp/node10 ordering export)

# Create a bls signer to sign transactions
crypto bls signer new --save private.key
crypto bls signer read --path private.key --format BASE64

# Authorize the signer to handle the access contract on each node
memcoin --config /tmp/node1 access add --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)
memcoin --config /tmp/node2 access add --identity $(crypto bls signer read --path private.key --format BASE64_PUBKEY)

# Update the access contract to allow us to use the value contract
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Access\
    --args access:grant_id --args 0200000000000000000000000000000000000000000000000000000000000000\
    --args access:grant_contract --args go.dedis.ch/dela.Value\
    --args access:grant_command --args all\
    --args access:identity --args $(crypto bls signer read --path private.key --format BASE64_PUBKEY)\
    --args access:command --args GRANT

memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:key --args "aef123"\
    --args value:value --args "Hello world"\
    --args value:command --args LIST\
    --nonce 34
```

memcoin --config /tmp/node1 proxy start --clientaddr 127.0.0.1:8081 && memcoin --config /tmp/node1 gapi register
memcoin --config /tmp/node2 proxy start --clientaddr 127.0.0.1:8082 && memcoin --config /tmp/node2 gapi register
memcoin --config /tmp/node3 proxy start --clientaddr 127.0.0.1:8083 && memcoin --config /tmp/node3 gapi register
memcoin --config /tmp/node4 proxy start --clientaddr 127.0.0.1:8084 && memcoin --config /tmp/node4 gapi register
memcoin --config /tmp/node5 proxy start --clientaddr 127.0.0.1:8085 && memcoin --config /tmp/node5 gapi register
memcoin --config /tmp/node6 proxy start --clientaddr 127.0.0.1:8086 && memcoin --config /tmp/node6 gapi register
memcoin --config /tmp/node7 proxy start --clientaddr 127.0.0.1:8087 && memcoin --config /tmp/node7 gapi register
memcoin --config /tmp/node8 proxy start --clientaddr 127.0.0.1:8088 && memcoin --config /tmp/node8 gapi register
memcoin --config /tmp/node9 proxy start --clientaddr 127.0.0.1:8089 && memcoin --config /tmp/node9 gapi register
memcoin --config /tmp/node10 proxy start --clientaddr 127.0.0.1:8010 && memcoin --config /tmp/node10 gapi register


memcoin --config /tmp/node1 pool add --key private.key\
    --args "use-unikernel" --args "yes"\
    --args tcp:addr --args 192.168.232.128:12345
