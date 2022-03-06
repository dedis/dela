#!/usr/bin/env bash

# This script list values from the blockchain.

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "${GREEN}[LIST]${NC} list all values"
memcoin --config /tmp/node1 pool add\
    --key private.key\
    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Value\
    --args value:command --args LIST
