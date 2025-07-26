#!/bin/bash
# Integration load test 1
for j in {1..5}; do
    echo "[LOAD-1] Processing batch $j"
    sleep 0.05
    if [ $((RANDOM % 10)) -lt 8 ]; then
        echo "[LOAD-1] Batch $j completed successfully"
    else
        echo "[LOAD-1] Batch $j failed"
        exit 1
    fi
done
echo "[LOAD-1] All batches completed successfully"
