#!/usr/bin/env bash
set -euo pipefail

### Globals
TMPDIR=""
PROXY_PY_PID=""
S1_PID=""
S2_PID=""
DP_PID=""

cleanup() {
    echo "Cleaning up..."
    kill "$PROXY_PY_PID" "$S1_PID" "$S2_PID" "$DP_PID" 2>/dev/null || true
    [[ -n "$TMPDIR" && -d "$TMPDIR" ]] && rm -rf "$TMPDIR"
    echo "Done."
}
trap cleanup EXIT

start_upstream_proxy() {
    python3 -m proxy --hostname 127.0.0.1 --port 8888 &
    PROXY_PY_PID=$!
    echo "Started proxy.py (PID: $PROXY_PY_PID) on 127.0.0.1:8888"
}

start_dummy_servers() {
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
}

start_go_proxy() {
    export UPSTREAM_PROXY="127.0.0.1:8888"
    export PROXY_EXCEPTIONS="127.0.0.1:8001"
    echo "Environment:"
    echo "  UPSTREAM_PROXY=$UPSTREAM_PROXY"
    echo "  PROXY_EXCEPTIONS=$PROXY_EXCEPTIONS"

    go build -o dynamicproxy cmd/main.go
    ./dynamicproxy &
    DP_PID=$!
    echo "Started Go proxy (PID: $DP_PID) on :8080"
}

run_tests() {
    sleep 1  # Give servers time to boot
    echo "=== TEST: server1 (bypass upstream) ==="
    curl -s -x http://127.0.0.1:8080 http://127.0.0.1:8001 || true
    echo

    echo "=== TEST: server2 (via upstream) ==="
    curl -s -x http://127.0.0.1:8080 http://127.0.0.1:8002 || true
    echo
}

main() {
    start_upstream_proxy
    start_dummy_servers
    start_go_proxy
    run_tests
}

main "$@"