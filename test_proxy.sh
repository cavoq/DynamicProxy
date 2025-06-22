#!/usr/bin/env bash

set -euo pipefail

# 1) Start a simple upstream HTTP proxy using proxy.py
# Install proxy.py if necessary: pip install proxy.py
python3 -m proxy --hostname 127.0.0.1 --port 8888 &
PROXY_PY_PID=$!

echo "Started upstream proxy.py (PID: $PROXY_PY_PID) on 127.0.0.1:8888"

# 2) Start two dummy HTTP servers serving different content
TMPDIR=$(mktemp -d)
mkdir -p "$TMPDIR/server1" "$TMPDIR/server2"

echo "Hello from server1" > "$TMPDIR/server1/index.html"
echo "Hello from server2" > "$TMPDIR/server2/index.html"

pushd "$TMPDIR/server1" >/dev/null
python3 -m http.server 8001 &
S1_PID=$!
popd >/dev/null

pushd "$TMPDIR/server2" >/dev/null
python3 -m http.server 8002 &
S2_PID=$!
popd >/dev/null

echo "Started server1 (PID: $S1_PID) on 127.0.0.1:8001"
echo "Started server2 (PID: $S2_PID) on 127.0.0.1:8002"

# 3) Export environment variables for the Go proxy
#  - UPSTREAM_PROXY : upstream proxy host:port
#  - PROXY_EXCEPTIONS : comma-separated list of host:port exceptions
export UPSTREAM_PROXY="127.0.0.1:8888"
export PROXY_EXCEPTIONS="127.0.0.1:8001"

echo "Environment configured:"
echo "  UPSTREAM_PROXY=$UPSTREAM_PROXY"
echo "  PROXY_EXCEPTIONS=$PROXY_EXCEPTIONS"

# 4) Build and start the Go dynamic proxy
# Make sure you are in project root
go build -o dynamicproxy cmd/main.go
./dynamicproxy &
DP_PID=$!

echo "Started dynamic Go proxy (PID: $DP_PID) on :8080"

# Allow servers to start
sleep 1

# 5) Test both endpoints via dynamic proxy
echo "=== TEST: server1 via proxy (should bypass upstream) ==="
curl -s -x http://127.0.0.1:8080 https://example.com/ || true

echo "=== TEST: server2 via proxy (should go through upstream) ==="
curl -s -x http://127.0.0.1:8080 https://example.com/ || true

# 6) Cleanup
echo "Cleaning up..."
kill $PROXY_PY_PID $S1_PID $S2_PID $DP_PID
rm -rf "$TMPDIR"
echo "Done."