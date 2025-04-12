#!/bin/sh -e

if ! [ -d /etc/avaron ]; then
	echo "configuration error: failed to find configuration directory - exitting" 1>&2
	exit 1
fi

# INTERNAL IP for the Avaron interface
if ! [ -f /etc/avaron/ip ]; then
	ip=10.7.4.1
elif ! read -r ip < /etc/avaron/ip; then
	echo "configuration error: failed to read avaron IP - exitting" 1>&2
	exit 1
fi

# INTERNAL NETMASK for the Avaron interface
if ! [ -f /etc/avaron/mask ]; then
	mask=24
elif ! read -r mask < /etc/avaron/mask; then
	echo "configuration error: failed to read avaron netmask - exitting" 1>&2
	exit 1
fi

# EXTERNAL port for the Avaron interface
if ! [ -f /etc/avaron/port ]; then
	port=10741
elif ! read -r port < /etc/avaron/ip; then
	echo "configuration error: failed to read avaron port number - exitting" 1>&2
	exit 1
fi

ip link add dev avaron type wireguard
ip address add "$ip/$mask" dev avaron
wg set avaron listen-port "$port" private-key /etc/avaron/key

find /var/lib/avaron/peers -type f | while read -r peer; do
	# HOSTNAME or EXTERNAL IP
	if ! read -r host; then
		echo "configuration error: unable to find external IP or hostname for peer $peer" 1>&2
		continue
	fi < "$peer/host"

	if ! read -r port; then
		port=10731
	fi < "$peer/port"

	wg set wg0 peer "$peer" endpoint "$host:$port"
	#ip route add "$ip"/32 dev wg0
done
