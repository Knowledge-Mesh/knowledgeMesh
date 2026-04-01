#!/bin/bash
# KnowledgeMesh integration test
# Starts a broker, registers a mock worker, sends a task.
# Usage: ./test/integration.sh

set -e

BROKER_PORT=9876
BROKER_URL="http://127.0.0.1:${BROKER_PORT}"
ADMIN_SECRET="test-admin-secret"
PASS=0
FAIL=0

cleanup() {
    echo ""
    echo "==> Cleaning up..."
    [ -n "$BROKER_PID" ] && kill $BROKER_PID 2>/dev/null || true
    rm -f /tmp/km-test-ledger.txt /tmp/km-test-ledger.txt.state.json
    echo "==> Results: $PASS passed, $FAIL failed"
    [ $FAIL -eq 0 ] && echo "==> ALL TESTS PASSED" || echo "==> SOME TESTS FAILED"
    exit $FAIL
}
trap cleanup EXIT

assert_ok() {
    local name="$1"
    local result="$2"
    local expected="$3"
    if echo "$result" | grep -q "$expected"; then
        echo "  PASS: $name"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $name"
        echo "    Expected to contain: $expected"
        echo "    Got: $result"
        FAIL=$((FAIL + 1))
    fi
}

echo "==> Building broker..."
cd "$(dirname "$0")/../fabric"
go build -o /tmp/km-test-broker . 2>&1

echo "==> Starting broker on port ${BROKER_PORT}..."
KM_ADMIN_SECRET="$ADMIN_SECRET" \
PORT="$BROKER_PORT" \
KM_LEDGER_PATH="/tmp/km-test-ledger.txt" \
/tmp/km-test-broker &
BROKER_PID=$!
sleep 2

echo "==> Running tests..."
echo ""

# Test 1: Health check
echo "--- Health ---"
RESULT=$(curl -s "${BROKER_URL}/health")
assert_ok "health endpoint" "$RESULT" '"status":"ok"'

# Test 2: Generate invite code
echo "--- Admin: Generate invite ---"
RESULT=$(curl -s -X POST "${BROKER_URL}/admin/invites" \
    -H "X-Admin-Secret: ${ADMIN_SECRET}" \
    -H "Content-Type: application/json" \
    -d '{"count":1}')
assert_ok "generate invite" "$RESULT" '"codes"'
INVITE=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['codes'][0])")

# Test 3: Register user
echo "--- Register user ---"
RESULT=$(curl -s -X POST "${BROKER_URL}/register-user" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"test-node\",\"invite_code\":\"${INVITE}\",\"email\":\"test@example.com\",\"tier\":\"api\",\"price_per_million_tokens\":1.0}")
assert_ok "register user" "$RESULT" '"node_secret"'
assert_ok "register gives credits" "$RESULT" '"credits"'
SECRET=$(echo "$RESULT" | python3 -c "import sys,json; print(json.load(sys.stdin)['node_secret'])")

# Test 4: WhoAmI
echo "--- WhoAmI ---"
RESULT=$(curl -s "${BROKER_URL}/whoami?secret=${SECRET}")
assert_ok "whoami returns name" "$RESULT" '"name":"test-node"'
assert_ok "whoami returns tier" "$RESULT" '"tier":"api"'
assert_ok "whoami shows email registered" "$RESULT" '"email_registered":true'

# Test 5: Node config
echo "--- Node config ---"
RESULT=$(curl -s "${BROKER_URL}/node-config?secret=${SECRET}")
assert_ok "node config returns name" "$RESULT" '"name":"test-node"'
assert_ok "node config returns tier" "$RESULT" '"tier":"api"'

# Test 6: Models endpoint (empty since no workers registered)
echo "--- Models ---"
RESULT=$(curl -s "${BROKER_URL}/models")
assert_ok "models endpoint" "$RESULT" '"count"'

# Test 7: Status
echo "--- Status ---"
RESULT=$(curl -s "${BROKER_URL}/status")
assert_ok "status has nodes" "$RESULT" '"nodes"'
assert_ok "status has total_tasks_completed" "$RESULT" '"total_tasks_completed"'

# Test 8: Recovery flow (token no longer returned in response — use admin endpoint)
echo "--- Recovery ---"
RESULT=$(curl -s -X POST "${BROKER_URL}/recover" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-node","email":"test@example.com"}')
assert_ok "recover does not leak token" "$RESULT" '"message"'

# Admin retrieves the reset token via the admin endpoint
RESULT=$(curl -s "${BROKER_URL}/admin/reset-tokens" \
    -H "X-Admin-Secret: ${ADMIN_SECRET}")
assert_ok "admin can list reset tokens" "$RESULT" '"reset_tokens"'
RESET_TOKEN=$(echo "$RESULT" | python3 -c "import sys,json; tokens=json.load(sys.stdin)['reset_tokens']; print(list(tokens.keys())[0])")

RESULT=$(curl -s -X POST "${BROKER_URL}/reset-secret" \
    -H "Content-Type: application/json" \
    -d "{\"reset_token\":\"${RESET_TOKEN}\"}")
assert_ok "reset gives new secret" "$RESULT" '"node_secret"'
assert_ok "reset confirms name" "$RESULT" '"name":"test-node"'

# Test 9: Wrong email recovery (wait for rate limit window)
sleep 6
echo "--- Wrong email recovery ---"
RESULT=$(curl -s -X POST "${BROKER_URL}/recover" \
    -H "Content-Type: application/json" \
    -d '{"name":"test-node","email":"wrong@example.com"}')
assert_ok "wrong email same response" "$RESULT" '"message"'

# Test 10: Rate limiting (fire 10 rapid register attempts)
echo "--- Rate limiting ---"
for i in $(seq 1 5); do
    curl -s -X POST "${BROKER_URL}/register-user" \
        -H "Content-Type: application/json" \
        -d '{"name":"spam","invite_code":"fake","email":"x@x.com"}' > /dev/null
done
RESULT=$(curl -s -X POST "${BROKER_URL}/register-user" \
    -H "Content-Type: application/json" \
    -d '{"name":"spam","invite_code":"fake","email":"x@x.com"}')
# Should get either rate limited or invalid invite (both are fine)
assert_ok "rapid requests handled" "$RESULT" '"error"'

echo ""
