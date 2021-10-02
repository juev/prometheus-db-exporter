VERSION        ?= 0.4.0

build: docker

deps:
	@PKG_CONFIG_PATH=${PWD} go get

docker:
	docker build -t "prometheus-db-exporter:$(VERSION)" .
	docker tag "prometheus-db-exporter:$(VERSION)" "prometheus-db-exporter:latest"

.PHONY: build deps docker