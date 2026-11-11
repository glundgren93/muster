#!/bin/sh
set -eu
set -- $MUSTER_AGENT

rex send -s "$1" -b "$2" -- "$MUSTER_MESSAGE"
rex send-key -s "$1" -b "$2" enter
