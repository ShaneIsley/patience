#!/bin/bash

# Diophantine Strategy - Proactive Rate Limit Compliance Examples
# This script demonstrates how to use the Diophantine strategy for optimal throughput
# while respecting rate limits through mathematical modeling

set -e

PATIENCE_BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Diophantine Strategy - Proactive Rate Limit Examples ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo

# Example 1: API with strict rate limits (GitHub API style)
echo "Example 1: GitHub API Rate Limiting (5000 requests/hour)"
echo "Simulating batch operations with proactive scheduling..."

# Simulate checking if we can schedule a new API call
# Rate limit: 5000 requests per hour, retry pattern: immediate, 10min, 30min
echo "Checking if we can schedule API call with existing load..."
$PATIENCE_BINARY diophantine \
    --rate-limit 5000 \
    --window 1h \
    --retry-offsets 0,10m,30m \
    --dry-run \
    -- echo "GitHub API call would be scheduled"

echo

# Example 2: High-frequency operations (Twitter API style)
echo "Example 2: Twitter API Rate Limiting (300 requests/15min)"
echo "Optimizing tweet posting with tight rate limits..."

# Rate limit: 300 requests per 15 minutes, retry pattern: immediate, 1min, 5min
$PATIENCE_BINARY diophantine \
    --rate-limit 300 \
    --window 15m \
    --retry-offsets 0,1m,5m \
    --attempts 3 \
    -- echo "Tweet posted successfully"

echo

# Example 3: Database operations with connection limits
echo "Example 3: Database Connection Pool (10 connections/minute)"
echo "Managing database operations to prevent connection exhaustion..."

# Rate limit: 10 connections per minute, retry pattern: immediate, 15s, 45s
$PATIENCE_BINARY diophantine \
    --rate-limit 10 \
    --window 1m \
    --retry-offsets 0,15s,45s \
    --attempts 3 \
    -- echo "Database query executed"

echo

# Example 4: Batch processing optimization
echo "Example 4: Batch Processing (100 operations/10min)"
echo "Maximizing throughput for batch operations..."

# Rate limit: 100 operations per 10 minutes, retry pattern: immediate, 2min, 5min
$PATIENCE_BINARY diophantine \
    --rate-limit 100 \
    --window 10m \
    --retry-offsets 0,2m,5m \
    --attempts 3 \
    -- echo "Batch operation completed"

echo

# Example 5: Microservice communication
echo "Example 5: Microservice Rate Limiting (50 requests/5min)"
echo "Coordinating service-to-service communication..."

# Rate limit: 50 requests per 5 minutes, retry pattern: immediate, 30s, 2min
$PATIENCE_BINARY diophantine \
    --rate-limit 50 \
    --window 5m \
    --retry-offsets 0,30s,2m \
    --attempts 3 \
    -- echo "Microservice call completed"

echo

echo "=== Diophantine Strategy Benefits ==="
echo "✓ Proactive rate limit compliance"
echo "✓ Mathematical precision using Diophantine inequalities"
echo "✓ Optimal throughput within constraints"
echo "✓ Prevents rate limit violations before they occur"
echo "✓ Ideal for controlled scheduling environments"
echo
echo "Use cases:"
echo "- API rate limit compliance"
echo "- Database connection management"
echo "- Batch processing optimization"
echo "- Microservice coordination"
echo "- Any scenario where you control task timing"