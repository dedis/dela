#! /bin/sh

# This script kills the tmux session started in teststart.sh and
# removes all the data pertaining to the test.

tmux kill-session -t dela-nodes-test && rm -rf /tmp/node*
