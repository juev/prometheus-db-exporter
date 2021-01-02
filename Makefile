VERSION        ?= 0.1.2
ORACLE_VERSION ?= 19.5
LDFLAGS        := -X main.Version=$(VERSION)
GOFLAGS        := -ldflags "$(LDFLAGS) -s -w"
ARCH           ?= $(shell uname -m)
GOARCH         ?= $(subst x86_64,amd64,$(patsubst i%86,386,$(ARCH)))
ORA_ZIP         = instantclient-basic-linux.x64-19.5.0.0.0dbru.zip
LD_LIBRARY_PATH = /usr/lib/oracle/$(ORACLE_VERSION)/client64/lib
BUILD_ARGS      = --build-arg VERSION=$(VERSION) --build-arg ORACLE_VERSION=$(ORACLE_VERSION)

%.zip:
	wget -q https://download.oracle.com/otn_software/linux/instantclient/195000/$@

download: $(ORA_ZIP)

build: download docker

clean:
	rm -rf ./prometheus-db-exporter

docker:
	docker build $(BUILD_ARGS) -t "juev/prometheus-db-exporter:$(VERSION)" .
	docker tag "juev/prometheus-db-exporter:$(VERSION)" "juev/prometheus-db-exporter:latest"

.PHONY: build clean docker download