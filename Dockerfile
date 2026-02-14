FROM golang:1.24-alpine AS deps

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

FROM deps AS builder

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o dynamicproxy cmd/main.go

FROM deps AS test
COPY . .
RUN go test -v ./...

FROM gcr.io/distroless/static-debian12

COPY --from=builder /build/dynamicproxy /dynamicproxy

COPY --from=deps /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 10001:10001

EXPOSE 8080

ENTRYPOINT ["/dynamicproxy"]
