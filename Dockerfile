FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY . .

RUN go mod tidy && go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o dynamicproxy cmd/main.go

FROM builder AS test
RUN go test -v ./...

FROM gcr.io/distroless/static-debian12

COPY --from=builder /build/dynamicproxy /dynamicproxy

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 10001:10001

EXPOSE 8080

ENTRYPOINT ["/dynamicproxy"]
