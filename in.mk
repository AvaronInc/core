.SUFFIXES: .jsx .mjs .js

.jsx.mjs:
	$(ESC) $(ESCFLAGS) $< > $@ || (rm -f $@; exit 1)

.mjs.js:
	$(ESL) $(ESLFLAGS) $< > $@ || (rm -f $@; exit 1)

SRC=\
	public/addressing/index.js \
	public/aim/index.js \
	public/containers/index.js \
	public/dashboard/index.js \
	public/devices/index.js \
	public/dns/index.js \
	public/firewall/index.js \
	public/logs/index.js \
	public/security/index.js \
	public/services/index.js \
	public/settings/index.js \
	public/storage/index.js \
	public/topology/index.js \
	public/version-control/index.js

build: \
	$(SRC) \
	$(BIN).service \
	$(BIN).rules \
	$(BIN)

serve: build
	./avaron

$(SRC): public/frame.mjs

$(BIN): $(GO_FILES)
	go build

run: build
	./$(BIN)

$(BIN).service: in.service Makefile
	sed 's,@PREFIX,$(PREFIX),g; s,@BIN,$(BIN),g' in.service > $@

node_modules: package.json
	$(NPM) i && touch -c node_modules

$(BIN).rules: in.rules Makefile
	sed 's,@BIN,$(BIN),g' in.rules > $@

install: build
	mkdir -p $(PREFIX)/lib/systemd/system/
	cp -f $(BIN) $(PREFIX)/bin/$(BIN)
	mkdir -p $(PREFIX)/lib/systemd/system/
	cp -f $(BIN).service $(PREFIX)/lib/systemd/system/$(BIN).service
	cp -f $(BIN).rules /etc/polkit-1/rules.d/$(BIN).rules

	printf "%s ALL=(ALL) !ALL\n" "$(BIN)"  > "/etc/sudoers.d/$(BIN)"
	printf "%s ALL=(ALL) NOPASSWD: /usr/sbin/ip, /usr/bin/wg, /usr/sbin/ethtool, /usr/local/sbin/ethtool\n" "$(BIN)" >> "/etc/sudoers.d/$(BIN)"

	if id $(BIN) >/dev/null 2>&1; then \
		printf "user %s already exists - not recreating\n" $(BIN); \
	else \
		(grep ^ssl: /etc/group >/dev/null || groupadd ssl) && \
		useradd -m $(BIN) -s /bin/sh -r -G ssl,video,render && \
		su $(BIN) sh -c 'cd && yes "" | ssh-keygen && mkdir -p peers wireguard' && \
		su $(BIN) sh -c 'cd ~/wireguard && touch private && chmod 600 private && chown $(BIN) private' && \
		su $(BIN) sh -c 'cd ~/wireguard && wg genkey | tee private | wg pubkey > public' && \
		su $(BIN) sh -c 'cd ~/wireguard && chmod 400 private'; \
	fi

uninstall:
	rm -f /etc/$(BIN)/key \
		$(PREFIX)/lib/systemd/$(BIN).service \
		$(PREFIX)/var/lib/$(BIN)/key ||:
	rm -rf $(PREFIX)/var/lib/$(BIN)/peers ||:
	rmdir /etc/$(BIN) ||:
	rmdir $(PREFIX)/var/lib/$(BIN) ||:
	userdel -rf $(BIN)

restart:
	systemctl daemon-reload
	systemctl restart $(BIN)

clean:
	go clean
	rm -f $(BIN).service public/*.js public/*.mjs  public/*/*.js public/*/*.mjs

nuke: clean
	rm -f Makefile
