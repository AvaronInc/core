#!/bin/sh -e

cd ~

# INTERNAL IP for the Avaron interface
if ! [ -f address ]; then
	address=10.7.4.1
elif ! read -r address < address; then
	echo "configuration error: failed to read avaron IP - exitting" 1>&2
	exit 1
fi

# INTERNAL NETMASK for the Avaron interface
if ! [ -f mask ]; then
	mask=24
elif ! read -r mask < mask; then
	echo "configuration error: failed to read avaron netmask - exitting" 1>&2
	exit 1
fi

# EXTERNAL port for the Avaron interface
if ! [ -f port ]; then
	port=10741
elif ! read -r port < address; then
	echo "configuration error: failed to read avaron port number - exitting" 1>&2
	exit 1
fi

sudo ip link    add dev avaron type        wireguard
sudo ip address add dev avaron             "$address/$mask"
sudo wg         set     avaron listen-port "$port"     private-key key

(
	cd peers
	find -type f | while read -r peer; do
		# HOSTNAME or EXTERNAL IP
		if ! read -r host; then
			echo "configuration error: unable to find external IP or hostname for peer $peer" 1>&2
			continue
		fi < "$peer/host"

		if ! read -r port; then
			port=58210
		fi < "$peer/port"

		sudo wg set wg0 peer "$peer" endpoint "$host:$port"
		#ip route add "$ip"/32 dev wg0
	done
)
