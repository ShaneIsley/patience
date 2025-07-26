#!/bin/bash
# Integration load test 3
for j in {1..5}; do
    echo "[LOAD-3] Processing batch $j"
    sleep 0.05
    if [ $((RANDOM % 10)) -lt 8 ]; then
        echo "[LOAD-3] Batch $j completed successfully"
    else
        echo "[LOAD-3] Batch $j failed"
        exit 1
    fi
done
echo "[LOAD-3] All batches completed successfully"
