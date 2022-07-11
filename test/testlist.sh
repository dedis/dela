#!/usr/bin/env bash

# This script lists all values on the blockchain.

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${GREEN}[LIST]${NC} list all values via node $1 using nonce $2"
memcoin --config /tmp/node$1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:command --args LIST\
    --nonce $2
