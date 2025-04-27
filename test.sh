#!/bin/sh -e

if [ -n "$SSH_CONNECTION" ]; then
	set $SSH_CONNECTION
fi

if [ -n "$1" ]; then
	peer=$1
elif [ -z "$peer" ]; then
	echo peer undefined
	exit 1
fi

./reinstall.sh

sudo su avaron -c sh <<- EOF
avaron &
sleep 1
avaron peer $peer:8080
sleep 10
EOF
