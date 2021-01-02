FROM golang AS build

WORKDIR /go/src/prometheus_postgres_exporter
COPY . .
RUN go get -d -v

ARG VERSION
ENV VERSION ${VERSION:-0.1.0}

ENV PKG_CONFIG_PATH /go/src/prometheus_postgres_exporter
ENV GOOS            linux

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags "-X main.Version=${VERSION} -s -w"

#upx
FROM gruebel/upx:latest as upx
COPY --from=build /go/src/prometheus_postgres_exporter/prometheus_postgres_exporter /prometheus_postgres_exporter.org

# Compress the binary and copy it to final image
RUN upx --best --lzma -o /prometheus_postgres_exporter /prometheus_postgres_exporter.org

# new stage
FROM scratch
LABEL authors="Denis Evsyukov"
LABEL maintainer="Denis Evsyukov <denis@evsyukov.org>"

ENV VERSION ${VERSION:-0.1.0}

COPY --from=build /go/src/prometheus_postgres_exporter/prometheus_postgres_exporter /entrypoint

EXPOSE 9102

ENTRYPOINT ["/entrypoint"]