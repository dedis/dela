# DKGCLI

DKGCLI is a CLI tool for using the DKG protocol. Here is a complete scenario:

```sh
# Install the CLI
go install .

# Run 3 nodes. Do that in 3 different sessions
LLVL=info dkgcli --config /tmp/node1 start --routing tree --listen tcp://127.0.0.1:2001
LLVL=info dkgcli --config /tmp/node2 start --routing tree --listen tcp://127.0.0.1:2002
LLVL=info dkgcli --config /tmp/node3 start --routing tree --listen tcp://127.0.0.1:2003

# Exchange certificates
dkgcli --config /tmp/node2 minogrpc join --address //127.0.0.1:2001 $(dkgcli --config /tmp/node1 minogrpc token)
dkgcli --config /tmp/node3 minogrpc join --address //127.0.0.1:2001 $(dkgcli --config /tmp/node1 minogrpc token)

# Initialize DKG on each node. Do that in a 4th session.
dkgcli --config /tmp/node1 dkg listen
dkgcli --config /tmp/node2 dkg listen
dkgcli --config /tmp/node3 dkg listen

# Do the setup in one of the node:
dkgcli --config /tmp/node1 dkg setup \
    --authority $(cat /tmp/node1/dkgauthority) \
    --authority $(cat /tmp/node2/dkgauthority) \
    --authority $(cat /tmp/node3/dkgauthority)

# Encrypt a message:
dkgcli --config /tmp/node2 dkg encrypt --message deadbeef

# Decrypt a message
dkgcli --config /tmp/node3 dkg decrypt --encrypted <...>