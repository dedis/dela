#!/bin/sh

# This script creates a new tmux session and starts nodes according to the
# instructions is README.md. The test session can be killed with teststop.sh.

set -o errexit

command -v tmux >/dev/null 2>&1 || { echo >&2 "tmux is not on your PATH!"; exit 1; }

# Launch session
s="dela-nodes-test"

tmux list-sessions | rg "^$s:" >/dev/null 2>&1 && { echo >&2 "A session with the name $s already exists; kill it and try again"; exit 1; }

tmux new -s $s -d

tmux split-window -t $s -h
tmux split-window -t $s:0.%1
tmux split-window -t $s:0.%2
tmux split-window -t $s:0.%3

# session s, window 0, panes 0 to 4
master="tmux send-keys -t $s:0.%0"
node1="tmux send-keys -t $s:0.%1"
node2="tmux send-keys -t $s:0.%2"
node3="tmux send-keys -t $s:0.%3"
node4="tmux send-keys -t $s:0.%4"

$node1 "LLVL=info memcoin --config /tmp/node1 start --listen //127.0.0.1:2001" C-m
$node2 "LLVL=info memcoin --config /tmp/node2 start --listen //127.0.0.1:2002" C-m
$node3 "LLVL=info memcoin --config /tmp/node3 start --listen //127.0.0.1:2003" C-m
$node4 "LLVL=info memcoin --config /tmp/node4 start --listen //127.0.0.1:2004" C-m

tmux select-pane -t 0

$master "./testsetup.sh" C-m

tmux a
