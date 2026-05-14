FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /estro ./cmd/estro

FROM alpine:3.23
RUN : \
  && apk add --no-cache openssh-client shadow su-exec \
  && useradd -m -d /app app
COPY --from=builder /estro /usr/local/bin/estro
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 3000
WORKDIR /app
ENTRYPOINT ["/entrypoint.sh"]
CMD ["estro", "-config", "config.yaml"]