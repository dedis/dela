#!/usr/bin/env bash

# This script stores a key/value pair on the blockchain.

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${GREEN}[STORE]${NC} store [key $1: value $2] via node $3 using nonce $4"
memcoin --config /tmp/node$3 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:key --args "key"$1\
    --args value:value --args $2\
    --args value:command --args WRITE\
    --nonce $4
