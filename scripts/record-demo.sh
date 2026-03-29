#!/bin/bash
# Record the fcm demo with asciinema
#
# Prerequisites:
#   sudo apt install asciinema
#   fcm must be installed and init'd
#   An ubuntu-24.04 image should already be pulled (to avoid download wait)
#
# Usage:
#   ./scripts/record-demo.sh
#
# Then upload:
#   asciinema upload /tmp/fcm-demo.cast
#
# Or convert to SVG:
#   npm install -g svg-term-cli
#   svg-term --in /tmp/fcm-demo.cast --out docs/demo.svg --window --width 80 --height 24

set -e

CAST_FILE="/tmp/fcm-demo.cast"

# Pre-pull the image so the demo is fast
sudo fcm pull ubuntu-24.04 2>/dev/null

# Clean up any existing demo VM
sudo fcm delete demo --force 2>/dev/null || true

echo "Starting recording in 3 seconds..."
echo "The script will type commands automatically."
echo "Press Ctrl+C to abort."
sleep 3

# Record
asciinema rec "$CAST_FILE" -c '
  # Simulate typing with delays
  type_cmd() {
    echo -n "$ "
    for ((i=0; i<${#1}; i++)); do
      echo -n "${1:$i:1}"
      sleep 0.05
    done
    echo
    sleep 0.3
    eval "$1"
    sleep 1
  }

  clear
  echo "# fcm — The CLI for Firecracker"
  echo ""
  sleep 2

  type_cmd "sudo fcm run demo --image ubuntu-24.04"

  # We are now SSH'"'"'d into the VM
  sleep 1
  type_cmd "uname -a"
  sleep 1
  type_cmd "cat /etc/os-release | head -2"
  sleep 1
  type_cmd "exit"

  sleep 1
  type_cmd "sudo fcm list"
  sleep 1
  type_cmd "sudo fcm delete demo --force"
  sleep 2

  echo ""
  echo "# Done! 🎉"
  echo "# Install: curl -fsSL https://raw.githubusercontent.com/venkatamutyala/fcm/main/install.sh | sudo bash"
  sleep 3
'

echo ""
echo "Recording saved to: $CAST_FILE"
echo ""
echo "Upload:  asciinema upload $CAST_FILE"
echo "SVG:     svg-term --in $CAST_FILE --out docs/demo.svg --window"
