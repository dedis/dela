#!/usr/bin/env bash

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "${GREEN}[PK]${NC} setup access rights on each node"
crypto bls signer new --save private.key
crypto bls signer read --path private.key --format BASE64
