### Install

```
PATH=$HOME/.local/bin/:$PATH &&
	printf "deb [signed-by=%s] %s stable main\n" \
	/usr/share/keyrings/elasticsearch-keyring.gpg \
	https://artifacts.elastic.co/packages/9.x/apt |
		sudo tee /etc/apt/sources.list.d/elastic-9.x.list &&
	curl https://artifacts.elastic.co/GPG-KEY-elasticsearch | gpg --dearmor -o - |
		sudo tee /usr/share/keyrings/elasticsearch-keyring.gpg > /dev/null
	sudo apt-get update &&
	sudo apt-get -y install \
		apt-transport-https \
		autoconf \
		curl \
		elasticsearch \
		gcc \
		git \
		golang \
		iproute2 \
		iptables \
		libcurl4-openssl-dev \
		libglib2.0-dev \
		libjson-perl \
		libmaxminddb-dev \
		libmnl-dev \
		libnghttp2-dev \
		libnl-genl-3-dev
		libpcap-dev \
		libpcre2-dev \
		libsystemd-dev
		libtool \
		libwww-perl \
		libyaml-dev \
		libyara-dev \
		libzstd-dev \
		m4 \
		make \
		uuid-dev \
		wireguard \
	&& (
		if [ "$(node -v)" = "v18.6.0" ]; then :; else
			cd ~/.local && curl https://nodejs.org/dist/v18.16.0/node-v18.16.0-linux-x64.tar.xz |
				tar xJf  - --strip-components=1
		fi
	) &&
		(:||git clone --recurse-submodules https://github.com/AvaronInc/core) &&
	(
		cd core/contrib/ethtool &&
			./autogen.sh &&
			./configure --prefix=/usr &&
			make -j "$(nproc)" &&
			sudo make install
	) && (
		cd core/contrib/arkime &&
			./bootstrap.sh &&
			./configure &&
			make -j "$(nproc)" &&
			sudo "PATH=$PATH" make install
	) && (
		cd core &&
			./configure &&
			make -j &&
			sudo groupadd -f ssl &&
			sudo make install
	) && (
		sudo cp core/elasticsearch.yml /etc/elasticsearch &&
		sudo cp core/config.ini /opt/arkime/etc/config.ini &&
		sudo systemctl restart elasticsearch &&
		sudo mkdir -p /opt/arkime/raw &&
		sudo chown root:daemon /opt/arkime/raw &&
		sudo chmod 775 /opt/arkime/raw &&
		sudo cp $HOME/.local/bin/node /opt/arkime/bin/ &&
		./core/contrib/arkime/db/db.pl localhost:9200 init &&
		sudo /opt/arkime/bin/arkime_update_geo.sh &&
		sudo systemctl restart arkimecapture &&
		sudo systemctl restart arkimeviewer
	)
```
