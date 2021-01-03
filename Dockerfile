FROM golang AS build

RUN apt-get -qq update && apt-get install --no-install-recommends -qq libaio1 unzip
ADD https://download.oracle.com/otn_software/linux/instantclient/195000/instantclient-basic-linux.x64-19.5.0.0.0dbru.zip /
RUN unzip /instantclient-basic-linux.x64-19.5.0.0.0dbru.zip -d /

WORKDIR /go/src/prometheus-db-exporter
COPY . .
RUN go get -d -v

ENV PKG_CONFIG_PATH /go/src/prometheus-db-exporter
ENV GOOS            linux

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -ldflags "-s -w"

# new stage
FROM frolvlad/alpine-glibc
LABEL authors="Denis Evsyukov"
LABEL maintainer="Denis Evsyukov <denis@evsyukov.org>"

COPY --from=build /instantclient_19_5 /instantclient_19_5

RUN apk add --no-cache libaio

ENV LD_LIBRARY_PATH="/instantclient_19_5"

COPY --from=build /go/src/prometheus-db-exporter/prometheus-db-exporter /entrypoint

EXPOSE 9103

ENTRYPOINT ["/entrypoint"]