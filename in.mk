build: $(BIN) $(BIN).service

$(BIN): $(GO_FILES)
	go build

run: build
	./$(BIN)

$(BIN).service: in.service Makefile
	sed 's,@PREFIX,$(PREFIX),g; s,@BIN,$(BIN),g' in.service > $(BIN).service

install: build
	mkdir -p $(PREFIX)/lib/systemd/system/
	cp $(BIN) $(PREFIX)/bin/$(BIN)
	cp bin/* $(PREFIX)/bin/
	mkdir -p $(PREFIX)/lib/systemd/system/
	cp $(BIN).service $(PREFIX)/lib/systemd/system/$(BIN).service

	mkdir -p $(PREFIX)/var/lib/$(BIN)/ $(PREFIX)/var/lib/$(BIN)/peers

	if [ ! -d "/etc/$(BIN)" ]; then \
		mkdir -p /etc/$(BIN) && \
		chown root:root /etc/$(BIN) && \
		chmod 700 /etc/$(BIN) && \
		wg genkey | tee /etc/$(BIN)/key | wg pubkey > $(PREFIX)/var/lib/$(BIN)/key; \
	fi

uninstall:
	rm -rf /etc/$(BIN)/key \
		$(PREFIX)/lib/systemd/$(BIN).service \
		$(PREFIX)/var/lib/$(BIN)/peers \
		$(PREFIX)/var/lib/$(BIN)/key
	rmdir /etc/$(BIN)
	rmdir $(PREFIX)/var/lib/$(BIN)

restart:
	systemctl daemon-reload
	systemctl restart $(BIN)

clean:
	go clean

nuke:
	go clean
	rm -f Makefile $(BIN).service
