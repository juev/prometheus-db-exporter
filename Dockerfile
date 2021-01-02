FROM golang AS build

ARG ORACLE_VERSION
ENV ORACLE_VERSION=${ORACLE_VERSION}
ENV LD_LIBRARY_PATH "/usr/lib/oracle/${ORACLE_VERSION}/client64/lib"

RUN apt-get -qq update && apt-get install --no-install-recommends -qq libaio1 unzip
COPY instantclient-basic-linux.x64-19.5.0.0.0dbru.zip /
RUN unzip /instantclient-basic-linux.x64-19.5.0.0.0dbru.zip -d /

WORKDIR /go/src/prometheus_db_exporter
COPY . .
RUN go get -d -v

ARG VERSION
ENV VERSION ${VERSION:-0.1.0}

ENV PKG_CONFIG_PATH /go/src/prometheus_db_exporter
ENV GOOS            linux

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -ldflags "-X main.Version=${VERSION} -s -w"

# new stage
FROM frolvlad/alpine-glibc
LABEL authors="Denis Evsyukov"
LABEL maintainer="Denis Evsyukov <denis@evsyukov.org>"

ENV VERSION ${VERSION:-0.1.0}

COPY --from=build /instantclient_19_5 /

RUN apk add --no-cache libaio

ARG ORACLE_VERSION
ENV ORACLE_VERSION=${ORACLE_VERSION}
ENV LD_LIBRARY_PATH "/instantclient_19_5"

RUN sh -c "echo /instantclient_19_5 > /etc/ld.so.conf.d/oracle-instantclient.conf"
RUN ldconfig

COPY --from=build /go/src/prometheus_db_exporter/prometheus_db_exporter /entrypoint

EXPOSE 9103

ENTRYPOINT ["/entrypoint"]