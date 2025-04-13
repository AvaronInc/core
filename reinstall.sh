#!/bin/sh
if ! id="$(id -u)"; then
	echo "id command failed" 1>&2
	exit 1
elif [ "$id" = "" ] || [ "$id" -eq 0 ]; then
	echo "don't run as root" 1>&2
	exit 1
fi

make &&
	(sudo make uninstall||:) &&
	sudo make install
