#!/bin/bash
#
# tests/test_log_sanitization.sh
#
# Standalone test — run by hand against a running docker-compose stack.
# Verifies that sensitive values (passwords, tokens, secrets) never appear
# in plaintext in container logs at ANY log level.
#
# Prerequisites:
#   1. Stack is running:  LOG_LEVEL=debug bash docker-compose-files/compose.sh up --build -d
#      (debug ensures maximum log output — the test covers every level)
#   2. curl and jq are available
#
# Usage:
#   bash tests/test_log_sanitization.sh
#

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
TOP_DIR=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE="bash $TOP_DIR/docker-compose-files/compose.sh"

BASE_URL="https://localhost:443/api/v1"
CURL="curl -ksSf"

# Unique marker values — if any of these show up in plaintext in the logs,
# the sanitization is broken.
MARKER_PASS_IRC="MARKER_ircpass_t3st_4829"
MARKER_PASS_RAC="MARKER_racpass_t3st_7153"
MARKER_USER_IRC="ircuser_test"
MARKER_USER_RAC="racuser_test"
MARKER_IRC_IP="192.168.254.201"
MARKER_RAC_IP="192.168.254.202:443"

FAIL=0
TESTS=0
PASSED=0

pass() { TESTS=$((TESTS+1)); PASSED=$((PASSED+1)); echo "  PASS: $1"; }
fail() { TESTS=$((TESTS+1)); FAIL=$((FAIL+1));     echo "  FAIL: $1"; }

# ---------------------------------------------------------------
# 0. Preflight — make sure the stack is up
# ---------------------------------------------------------------
echo "=== Preflight ==="
if ! $CURL "$BASE_URL/Systems" >/dev/null 2>&1 &&
   ! $CURL "$BASE_URL/irc" -u admin:admin >/dev/null 2>&1; then
    echo "ERROR: Cannot reach $BASE_URL — is the stack running?"
    echo "       LOG_LEVEL=debug bash docker-compose-files/compose.sh up --build -d"
    exit 1
fi
echo "  Stack is reachable."

# ---------------------------------------------------------------
# 1. Authenticate (generates a JWT — the token value is also sensitive)
# ---------------------------------------------------------------
echo ""
echo "=== Generating traffic with sensitive data ==="

TOKEN_RESP=$($CURL -X POST -H "Content-Type: application/json" \
    -d "{\"username\":\"admin\",\"password\":\"admin\"}" \
    "$BASE_URL/GetToken" 2>/dev/null || true)

JWT=$(echo "$TOKEN_RESP" | jq -r '.token // empty' 2>/dev/null || true)
if [ -z "$JWT" ]; then
    echo "  (token auth unavailable, falling back to basic auth)"
    AUTH="-u admin:admin"
else
    AUTH="-H \"Authorization: Bearer $JWT\""
fi

# ---------------------------------------------------------------
# 2. Add an IRC with a marker password
# ---------------------------------------------------------------
echo "  Adding IRC ($MARKER_IRC_IP) with marker password..."
$CURL -X POST -u admin:admin -H "Content-Type: application/json" \
    -d "{\"ip\":\"$MARKER_IRC_IP\",\"username\":\"$MARKER_USER_IRC\",\"password\":\"$MARKER_PASS_IRC\"}" \
    "$BASE_URL/irc" >/dev/null 2>&1 || true

# ---------------------------------------------------------------
# 3. Add a rack device with a marker password
# ---------------------------------------------------------------
echo "  Adding rack device ($MARKER_RAC_IP) with marker password..."
$CURL -X POST -u admin:admin -H "Content-Type: application/json" \
    -d "{\"ip\":\"$MARKER_RAC_IP\",\"username\":\"$MARKER_USER_RAC\",\"password\":\"$MARKER_PASS_RAC\",\"systemtypedesc\":\"idrac\",\"isforcefulshutdown\":true}" \
    "$BASE_URL/irc/$MARKER_IRC_IP/racdevice" >/dev/null 2>&1 || true

# Small delay so log lines are flushed
sleep 3

# ---------------------------------------------------------------
# 4. Capture logs from ALL containers
# ---------------------------------------------------------------
echo ""
echo "=== Capturing container logs ==="
LOGS=$($COMPOSE logs --no-color 2>/dev/null)

# ---------------------------------------------------------------
# 5. Check that marker passwords never appear in plaintext
# ---------------------------------------------------------------
echo ""
echo "=== Checking logs for leaked sensitive values ==="

for marker in "$MARKER_PASS_IRC" "$MARKER_PASS_RAC"; do
    if echo "$LOGS" | grep -qF "$marker"; then
        fail "Password marker '$marker' found in plaintext in logs"
        echo "       Offending lines:"
        echo "$LOGS" | grep -F "$marker" | head -5 | sed 's/^/         /'
    else
        pass "Password marker '$marker' not found in logs"
    fi
done

# ---------------------------------------------------------------
# 6. Check that [REDACTED] appears (proves sanitization ran)
# ---------------------------------------------------------------
if echo "$LOGS" | grep -q '\[REDACTED\]'; then
    pass "[REDACTED] marker found in logs (sanitization is active)"
else
    fail "[REDACTED] marker NOT found — sanitization may not be running"
    echo "       Hint: make sure LOG_LEVEL=debug so Info-level logs are emitted"
fi

# ---------------------------------------------------------------
# 7. Check JWT token doesn't leak (if we got one)
# ---------------------------------------------------------------
if [ -n "$JWT" ]; then
    if echo "$LOGS" | grep -qF "$JWT"; then
        fail "JWT token found in plaintext in logs"
    else
        pass "JWT token not found in logs"
    fi
fi

# ---------------------------------------------------------------
# 8. Check that common secret env vars are not logged in plaintext
#    (MYSQL_PASSWORD, CONFIGUI_JWT_SECRET)
# ---------------------------------------------------------------
ENV_FILE="$TOP_DIR/docker-compose-files/.env"
if [ -f "$ENV_FILE" ]; then
    MYSQL_PW=$(grep '^MYSQL_PASSWORD=' "$ENV_FILE" | cut -d= -f2- || true)
    JWT_SECRET=$(grep '^CONFIGUI_JWT_SECRET=' "$ENV_FILE" | cut -d= -f2- || true)

    if [ -n "$MYSQL_PW" ]; then
        if echo "$LOGS" | grep -qF "$MYSQL_PW"; then
            fail "MYSQL_PASSWORD found in plaintext in logs"
        else
            pass "MYSQL_PASSWORD not found in logs"
        fi
    fi

    if [ -n "$JWT_SECRET" ]; then
        if echo "$LOGS" | grep -qF "$JWT_SECRET"; then
            fail "CONFIGUI_JWT_SECRET found in plaintext in logs"
        else
            pass "CONFIGUI_JWT_SECRET not found in logs"
        fi
    fi
fi

# ---------------------------------------------------------------
# 9. Cleanup — remove the test devices we added
# ---------------------------------------------------------------
echo ""
echo "=== Cleanup ==="
$CURL -X DELETE -u admin:admin -H "Content-Type: application/json" \
    -d "{\"ips\":[\"$MARKER_RAC_IP\"]}" \
    "$BASE_URL/irc/$MARKER_IRC_IP/racdevice" >/dev/null 2>&1 || true

$CURL -X DELETE -u admin:admin -H "Content-Type: application/json" \
    -d "{\"ips\":[\"$MARKER_IRC_IP\"]}" \
    "$BASE_URL/irc" >/dev/null 2>&1 || true
echo "  Test devices removed."

# ---------------------------------------------------------------
# Summary
# ---------------------------------------------------------------
echo ""
echo "==============================="
echo "  Results: $PASSED/$TESTS passed"
if [ "$FAIL" -gt 0 ]; then
    echo "  $FAIL FAILED"
    echo "==============================="
    exit 1
else
    echo "  All checks passed."
    echo "==============================="
    exit 0
fi
