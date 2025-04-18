build: $(BIN) $(BIN).service

$(BIN): $(GO_FILES)
	go build

run: build
	./$(BIN)

$(BIN).service: in.service Makefile
	sed 's,@PREFIX,$(PREFIX),g; s,@BIN,$(BIN),g' in.service > $(BIN).service

install: build
	if id $(BIN) >/dev/null 1>&2; then \
		echo "$(BIN) user already exists" 1>&2; \
		exit 1; \
	fi

	mkdir -p $(PREFIX)/lib/systemd/system/
	cp $(BIN) $(PREFIX)/bin/$(BIN)
	cp bin/* $(PREFIX)/bin/
	mkdir -p $(PREFIX)/lib/systemd/system/
	cp $(BIN).service $(PREFIX)/lib/systemd/system/$(BIN).service

	useradd -m $(BIN) -s /bin/sh -r

	printf "%s ALL=(ALL) !ALL\n"                       "$(BIN)"  > "/etc/sudoers.d/$(BIN)"
	printf "%s ALL=(ALL) NOPASSWD: /bin/ip, /bin/wg, /bin/ethtool\n" "$(BIN)" >> "/etc/sudoers.d/$(BIN)"

	su $(BIN) sh -c 'cd && yes "" | ssh-keygen && mkdir -p peers wireguard'
	su $(BIN) sh -c 'cd ~/wireguard && touch private && chmod 600 private && chown $(BIN) private'
	su $(BIN) sh -c 'cd ~/wireguard && wg genkey | tee private | wg pubkey > public'
	su $(BIN) sh -c 'cd ~/wireguard && chmod 400 private'
	cp bin/init.sh /tmp/init.sh
	su $(BIN) sh -c 'cd && mkdir -p .local/bin && cp /tmp/init.sh .local/bin/ && chmod 555 .local/bin/init.sh'

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

nuke:
	go clean
	rm -f Makefile $(BIN).service
