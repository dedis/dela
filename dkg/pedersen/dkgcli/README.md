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
```

# Use docker

Build an image if needed (from the root of the repo):

```sh
docker build -t dela/dkg:latest -f dkg/pedersen/dkgcli/dockerfile .
```

Create a network if not exist. We use a user-defined bridge to make use of
Docker's DNS service, which allows containers to communicate among themselves
using their names:

```sh
sudo docker network create dkg-net
```

Run 3 nodes:

```sh
for i in {1..3}
do
    sudo docker run --rm -d -e LLVL=info --expose 2000 --net dkg-net --name node$i dela/dkg:latest --config /config start --listen tcp://0.0.0.0:2000 --public //node$i:2000
done
```

Exchange certificates:

```sh
for i in {2..3}
do
    sudo docker exec node$i dkgcli --config /config minogrpc join --address //node1:2000 $(sudo docker exec node1 dkgcli --config /config minogrpc token)
done
```

Listen:

```sh
for i in {1..3}
do
    sudo docker exec node$i dkgcli --config /config dkg listen
done
```

Setup:

```sh
authorities=""

for i in {1..3}
do
    authorities="$authorities --authority $(sudo docker exec node$i cat /config/dkgauthority)"    
done

sudo docker exec node1 dkgcli --config /config dkg setup $authorities
```

Encrypt/Decrypt:

```sh
encrypted=$(sudo docker exec node2 dkgcli --config /config dkg encrypt --message deadbeef)
sudo docker exec node2 dkgcli --config /config dkg decrypt --encrypted $encrypted
```

Stop:

```sh
for i in {1..3}
do
    sudo docker stop node$i
done
```