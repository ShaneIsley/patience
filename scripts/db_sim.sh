#!/bin/bash
# Simulate database connection
attempt_file="/tmp/db_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 2 ]; then
    echo "[DB] Connection refused - database not ready"
    exit 1
elif [ $attempts -eq 3 ]; then
    echo "[DB] Connection established successfully"
    rm -f "$attempt_file"
    exit 0
else
    echo "[DB] Connection established successfully"
    exit 0
fi
