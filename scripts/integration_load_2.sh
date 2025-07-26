#!/bin/bash
# Integration load test 2
for j in {1..5}; do
    echo "[LOAD-2] Processing batch $j"
    sleep 0.05
    if [ $((RANDOM % 10)) -lt 8 ]; then
        echo "[LOAD-2] Batch $j completed successfully"
    else
        echo "[LOAD-2] Batch $j failed"
        exit 1
    fi
done
echo "[LOAD-2] All batches completed successfully"
