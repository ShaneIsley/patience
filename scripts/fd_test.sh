#!/bin/bash
echo "[FD] Testing file descriptor usage"
# Try to open multiple files
for i in {1..10}; do
    exec {fd}< /dev/null
    echo "[FD] Opened file descriptor $fd"
done
echo "[FD] File descriptor test completed"
