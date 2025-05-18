### Clone the Back-end

```
git clone --recurse-submodules https://github.com/AvaronInc/core
```

### Clone the Front-end

```
git clone --recurse-submodules https://github.com/AvaronInc/ui
```

### Install

```
PATH=$HOME/.local/bin/:$PATH &&
	(
		cd /usr/share/keyrings &&
		curl -L https://artifacts.elastic.co/GPG-KEY-elasticsearch |
			gpg --dearmor -o - |
			sudo tee elasticsearch-keyring.gpg &&
		curl -L https://apt.repos.intel.com/intel-gpg-keys/GPG-PUB-KEY-INTEL-SW-PRODUCTS.PUB |
			gpg --dearmor |
			sudo tee oneapi-archive-keyring.gpg &&
		cd /etc/apt/sources.list.d &&
		printf "deb [signed-by=%s] %s all main\n"  \
			/usr/share/keyrings/oneapi-archive-keyring.gpg \
			https://apt.repos.intel.com/oneapi |
				sudo tee oneAPI.list &&
		printf "deb [signed-by=%s] %s stable main\n" \
		/usr/share/keyrings/elasticsearch-keyring.gpg \
		https://artifacts.elastic.co/packages/9.x/apt |
			sudo tee elastic-9.x.list
	) > /dev/null &&
	sudo apt-get update &&
	sudo apt-get -y install \
		apt-transport-https \
		autoconf \
		curl \
		elasticsearch \
		gcc \
		git \
		golang \
		intel-basekit \
		intel-oneapi-runtime-libs \
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
	) && (
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
		cd core/contrib/llama.cpp &&
			mkdir build &&
			cd build &&
			set -- --force &&
			. /opt/intel/oneapi/2025.1/oneapi-vars.sh &&
			cmake -DGGML_BLAS_VENDOR=IntelONEAPI -DGGML_SYCL=ON -DCMAKE_C_COMPILER=icx -DCMAKE_CXX_COMPILER=icpx -DCMAKE_BUILD_TYPE=Release ..
			make -j "$(nproc)" &&
			sudo make install
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
