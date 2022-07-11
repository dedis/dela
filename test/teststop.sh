#! /bin/sh

set -e

GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${GREEN}[STOP]${NC} kills nodes"
rm -rf /tmp/node* 

sleep 1

tmux kill-session -t dela-nodes-test 
