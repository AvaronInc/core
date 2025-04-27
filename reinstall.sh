#!/bin/sh
if ! id="$(id -u)"; then
	echo "id command failed" 1>&2
	exit 1
elif [ "$id" = "" ] || [ "$id" -eq 0 ]; then
	echo "don't run as root" 1>&2
	exit 1
fi

sudo ip link del dev avaron ||:
./configure &&
	make &&
	(sudo killall avaron||:) &&
	(sudo make uninstall||:) &&
	sudo make install
