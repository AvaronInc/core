### Dependencies

```
sudo apt-get -y install \
	 autoconf \
	 automake \
	 gcc \
	 git \
	 golang \
	 iproute2 \
	 iptables \
	 libmnl-dev \
	 libtool \
	 m4 \
	 make \
	 wireguard
```

On Ubuntu, we have to compile `ethtool` from source, for now:

```
wget https://www.kernel.org/pub/software/network/ethtool/ethtool-6.11.tar.xz &&
	xzcat ethtool-6.11.tar.xz | tar xvf - && (
		cd ethtool-6.11 &&
			./autogen.sh &&
			./configure --prefix=/usr &&
			make -j &&
			sudo make -j`nproc` install
	)
```

Reason being, the version shipped on Ubuntu 24.04 ship doesn't give JSON output - an oversight on my part.
Soon we may just query `/sys/class/net` & others.

### Avaron Core Installation

```
./configure && make && sudo make install
```

### Serve the web interface

Install dependencies:

```
sudo apt-get -y install nodejs
```

Clone the UI repo, install the node dependencies & build the directory to be served over HTTP:

```
git clone https://github.com/AvaronInc/ui && cd ui &&
	npm install . &&
	npm run build
```

Then copy the `dist` directory to `/tmp/public`:

```
cp -r dist /tmp/public
```

View in a web-browser:

```
chromium localhost:8080
```

### Add a peer

```
sudo -u avaron 'avaron peer 10.0.1.101:8080'
```
