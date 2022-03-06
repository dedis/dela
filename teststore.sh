#!/usr/bin/env bash

# This script store a value on the blockchain.

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "${GREEN}[STORE]${NC} store a [key $2: value $3] via node $1"
memcoin --config /tmp/node$1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:key --args "key"$2\
    --args value:value --args $3\
    --args value:command --args WRITE
