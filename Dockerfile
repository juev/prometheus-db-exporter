FROM golang:1.17 AS build

WORKDIR /go/src/app
COPY . .
ENV PKG_CONFIG_PATH /go/src/app
RUN set -ex \
  && go get -d -v \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -ldflags "-s -w" -o /app.bin \
  && go version \
  && ls -al /app.bin

FROM gcr.io/distroless/static
LABEL authors="Denis Evsyukov"
LABEL maintainer="Denis Evsyukov <denis@evsyukov.org>"
COPY --from=build /app.bin /entrypoint

EXPOSE 9103

ENTRYPOINT ["/entrypoint"]