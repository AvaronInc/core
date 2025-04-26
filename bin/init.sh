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
	port=51820
elif ! read -r port < address; then
	echo "configuration error: failed to read avaron port number - exitting" 1>&2
	exit 1
fi

if ! sudo ip addr show avaron; then
	sudo ip link add dev avaron type wireguard >/dev/null
fi | awk '($1 == "inet" || $1 == "inet6") { print $2 }' | while read -r addr; do
	sudo ip addr del "$addr" dev avaron
done

sudo ip address add     dev avaron             "$address/$mask"
sudo wg         set         avaron listen-port "$port"     private-key wireguard/private
sudo ip link    set up  dev avaron
sudo ip route add "$(avaron netmask "$address/$mask")" dev avaron src "$address"||:

(
	cd links
	for link in *; do
		# HOSTNAME or EXTERNAL IP
		if ! read -r host; then
			echo "configuration error: unable to find external IP or hostname for link $link" 1>&2
			continue
		fi < "$link/host"

		# INTERNAL NETMASK for the Avaron interface
		if ! [ -f "$link/port" ]; then
			port=51820
		elif ! read -r  < "$link/port"; then
			echo "configuration error: failed to read port for $link" 1>&2
			exit 1
		fi

		sudo wg set avaron link "$(echo "$link" | tr '-' '/')" endpoint "$host:$port" allowed-ips 0.0.0.0/0
	done
)
