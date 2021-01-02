VERSION        ?= 0.1.0
ORACLE_VERSION ?= 19.5
LDFLAGS        := -X main.Version=$(VERSION)
GOFLAGS        := -ldflags "$(LDFLAGS) -s -w"
ARCH           ?= $(shell uname -m)
GOARCH         ?= $(subst x86_64,amd64,$(patsubst i%86,386,$(ARCH)))
RPM_VERSION    ?= $(ORACLE_VERSION).0.0.0-1
ORA_RPM         = oracle-instantclient$(ORACLE_VERSION)-devel-$(RPM_VERSION).$(ARCH).rpm oracle-instantclient$(ORACLE_VERSION)-basic-$(RPM_VERSION).$(ARCH).rpm
ORA_ZIP         = instantclient-basic-linux.x64-19.5.0.0.0dbru.zip
LD_LIBRARY_PATH = /usr/lib/oracle/$(ORACLE_VERSION)/client64/lib
BUILD_ARGS      = --build-arg VERSION=$(VERSION) --build-arg ORACLE_VERSION=$(ORACLE_VERSION)
DIST_DIR        = prometheus_oracle.$(VERSION)-ora$(ORACLE_VERSION).linux-${GOARCH}
ARCHIVE         = prometheus_oracle.$(VERSION)-ora$(ORACLE_VERSION).linux-${GOARCH}.tar.gz

%.rpm:
	wget -q http://yum.oracle.com/repo/OracleLinux/OL7/oracle/instantclient/$(ARCH)/getPackage/$@

%.zip:
	wget -q https://download.oracle.com/otn_software/linux/instantclient/195000/$@

download-rpms: $(ORA_RPM)

download-zips: $(ORA_ZIP)

download: download-rpms download-zips

oci.pc:
	sed "s/@ORACLE_VERSION@/$(ORACLE_VERSION)/g" oci8.pc.template > oci8.pc

build: download docker

clean:
	rm -rf ./prometheus_db_exporter oci8.pc

docker: download
	docker build $(BUILD_ARGS) -t "juev/prometheus_db_exporter:$(VERSION)" .
	docker tag "juev/prometheus_db_exporter:$(VERSION)" "juev/prometheus_db_exporter:latest"

.PHONY: build clean docker download