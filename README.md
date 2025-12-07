# DynamicProxy

**Dynamic HTTP Forward Proxy with Upstream Proxy Switching**

## 🧭 Overview

DynamicProxy is a lightweight HTTP forward proxy designed for flexible upstream proxy switching based on request context. It was built to overcome limitations in environments where only a single static proxy can be configured — like in [Mapsui](https://github.com/Mapsui/Mapsui) with [GDAL](https://gdal.org/) on Windows.

This proxy enables both **internal** and **external** geo services to work seamlessly without reconfiguring or restarting your app. It dynamically routes requests through the appropriate upstream proxy or direct connection, based on rules you define.

---

## 💡 Use Case

While working with **Mapsui + GDAL** on Windows, we encountered a limitation: only **one proxy** could be configured globally, with no way to make exceptions. This was a problem because our application needed to access both:

- **Internal geo-services** (which required no proxy)
- **External geo-services** (which required a corporate proxy)

**DynamicProxy** solves this by acting as a smart middle layer:
- It listens as a local proxy.
- It inspects each request.
- Based on destination (or other rules), it forwards via the right upstream proxy — or bypasses it entirely.

## 🚀 Getting Started

To configure DynamicProxy, you need to set up the following environment variables:

- `LISTEN_ADDR`: The address where the proxy will listen for incoming requests (default: `*:8080`).
- `UPSTREAM_PROXY`: The upstream proxy address to use for external requests (e.g. `corporate.proxy:8080`).
- `PROXY_EXCEPTIONS`: A comma-separated list of hostnames or IPs that should bypass the upstream proxy (e.g. `localhost,somehost1,somehost2`).
- `PROXY_AUTH`: Optional authentication for the upstream proxy (currently only ntlm for windows is supported, e.g. `ntlm`).

You can then run the binary:

```bash
./DynamicProxy
```

## 🛠️ Building from Source

To build DynamicProxy from source, ensure you have Go 1.24.0 or later installed and run the following commands:

```bash
go mod tidy
go build -o dynamicproxy ./cmd/main.go
```

## 🐳 Docker

### Build the image

```bash
docker build -t dynamicproxy .
```

### Run the container

```bash
docker run -d -p 8080:8080 \
  -e UPSTREAM_PROXY=corporate.proxy:8080 \
  -e PROXY_EXCEPTIONS=localhost,internal.host \
  dynamicproxy
```

### Run tests in Docker

```bash
docker build --target test .
```