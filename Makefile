VERSION        ?= 0.1.9
LDFLAGS        := -X main.Version=$(VERSION)
GOFLAGS        := -ldflags "$(LDFLAGS) -s -w"
ARCH           ?= $(shell uname -m)
GOARCH         ?= $(subst x86_64,amd64,$(patsubst i%86,386,$(ARCH)))

build: docker

deps:
	@PKG_CONFIG_PATH=${PWD} go get

clean:
	rm -rf ./prometheus_postgres_exporter

docker: ubuntu-image

ubuntu-image:
	docker build $(BUILD_ARGS)  -t "juev/prometheus_postgres_exporter:$(VERSION)" .
	docker tag "juev/prometheus_postgres_exporter:$(VERSION)" "juev/prometheus_postgres_exporter:latest"

.PHONY: build deps test clean docker