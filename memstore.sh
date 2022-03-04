#!/usr/bin/env bash

# This script store a value on the blockchain.

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "${GREEN}[STORE]${NC} store a key/value"
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:key --args "key"$1\
    --args value:value --args $2\
    --args value:command --args WRITE
