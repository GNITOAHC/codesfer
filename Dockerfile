# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
# CGO off: modernc.org/sqlite is pure Go
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/gnitoahc/codesfer/pkg/version.Version=${VERSION}" \
    -o /codeserver ./cmd/codesfer-server/main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /codeserver /usr/local/bin/codeserver
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:3000/ping || exit 1
ENTRYPOINT ["codeserver"]
CMD ["serve"]
