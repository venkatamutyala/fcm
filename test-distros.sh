#!/bin/bash
# FCM multi-distro test script
# Tests: pull → create → SSH (first boot) → stop → start → SSH (restart) → delete

set -uo pipefail

SSH_KEY="/home/demo/.ssh/id_ed25519"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes -o ConnectTimeout=10 -o LogLevel=ERROR"
WAIT_FIRST=60   # seconds to wait for first boot
WAIT_RESTART=60 # seconds to wait for restart boot

PASS=0
FAIL=0
SKIP=0
RESULTS=""

test_distro() {
    local NAME=$1
    local IMAGE=$2
    local VM="test-${NAME}"

    echo ""
    echo "=========================================="
    echo "  Testing: $NAME ($IMAGE)"
    echo "=========================================="

    # Pull if not cached
    if [ ! -f "/var/lib/fcm/images/${IMAGE}.ext4" ]; then
        echo "[pull] Downloading $IMAGE..."
        if ! fcm pull "$IMAGE" 2>&1; then
            echo "FAIL: pull failed"
            FAIL=$((FAIL+1))
            RESULTS="${RESULTS}\n  FAIL  $NAME — pull failed"
            return
        fi
    else
        echo "[pull] Already cached"
    fi

    # Create
    echo "[create] Creating VM..."
    if ! fcm create "$VM" --image "$IMAGE" --ssh-key "$SSH_KEY" 2>&1; then
        echo "FAIL: create failed"
        FAIL=$((FAIL+1))
        RESULTS="${RESULTS}\n  FAIL  $NAME — create failed"
        return
    fi

    # Wait and SSH (first boot)
    echo "[ssh] Waiting ${WAIT_FIRST}s for first boot..."
    sleep $WAIT_FIRST
    local IP=$(cat /var/lib/fcm/vms/$VM/vm.json 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['ip'])" 2>/dev/null)
    if [ -z "$IP" ]; then
        echo "FAIL: no IP in vm.json"
        fcm delete "$VM" --force 2>/dev/null
        FAIL=$((FAIL+1))
        RESULTS="${RESULTS}\n  FAIL  $NAME — no IP"
        return
    fi

    local FIRST_SSH
    FIRST_SSH=$(ssh $SSH_OPTS -i "$SSH_KEY" root@"$IP" "echo OK" 2>&1)
    if [ "$FIRST_SSH" != "OK" ]; then
        echo "FAIL: first boot SSH failed: $FIRST_SSH"
        fcm delete "$VM" --force 2>/dev/null
        FAIL=$((FAIL+1))
        RESULTS="${RESULTS}\n  FAIL  $NAME — first boot SSH failed"
        return
    fi
    echo "[ssh] First boot: OK"

    # Stop + Start
    echo "[restart] Stopping..."
    fcm stop "$VM" 2>&1
    sleep 2
    echo "[restart] Starting..."
    fcm start "$VM" 2>&1

    # Wait and SSH (restart)
    echo "[ssh] Waiting ${WAIT_RESTART}s for restart..."
    sleep $WAIT_RESTART
    local RESTART_SSH
    RESTART_SSH=$(ssh $SSH_OPTS -i "$SSH_KEY" root@"$IP" "echo OK" 2>&1)
    if [ "$RESTART_SSH" != "OK" ]; then
        echo "WARN: restart SSH failed (trying 30s more)..."
        sleep 30
        RESTART_SSH=$(ssh $SSH_OPTS -i "$SSH_KEY" root@"$IP" "echo OK" 2>&1)
    fi

    if [ "$RESTART_SSH" != "OK" ]; then
        echo "FAIL: restart SSH failed: $RESTART_SSH"
        fcm delete "$VM" --force 2>/dev/null
        FAIL=$((FAIL+1))
        RESULTS="${RESULTS}\n  FAIL  $NAME — restart SSH failed"
        return
    fi
    echo "[ssh] Restart: OK"

    # Get distro info
    local DISTRO_INFO
    DISTRO_INFO=$(ssh $SSH_OPTS -i "$SSH_KEY" root@"$IP" "cat /etc/os-release | head -1" 2>&1)
    echo "[info] $DISTRO_INFO"

    # Delete
    fcm delete "$VM" --force 2>&1
    echo "[done] $NAME: PASSED"
    PASS=$((PASS+1))
    RESULTS="${RESULTS}\n  PASS  $NAME — $DISTRO_INFO"
}

echo "FCM Multi-Distro Test Suite"
echo "==========================="
echo ""

# Test each distro (skip Fedora — mirrors are flaky)
test_distro "ubuntu2404" "ubuntu-24.04"
test_distro "ubuntu2204" "ubuntu-22.04"
test_distro "debian12"   "debian-12"
test_distro "rocky9"     "rocky-9"
test_distro "alma9"      "alma-9"
test_distro "centos9"    "centos-stream9"
test_distro "arch"       "arch"
test_distro "opensuse"   "opensuse-15.6"
test_distro "alpine"     "alpine-3.20"

echo ""
echo "=========================================="
echo "  RESULTS: $PASS passed, $FAIL failed, $SKIP skipped"
echo "=========================================="
echo -e "$RESULTS"
echo ""
