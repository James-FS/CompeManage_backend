#!/bin/bash

# wrk_load_test.sh - 压力测试脚本
# 用法: ./wrk_load_test.sh [URL] [持续时间]
# 示例: ./wrk_load_test.sh http://localhost:8080/api/award/list 30

URL="${1:-http://localhost:8080/api/award/list}"
DURATION="${2:-30s}"
THREADS=4

echo "========================================"
echo "WRK 压力测试"
echo "URL: $URL"
echo "Duration: $DURATION"
echo "Threads: $THREADS"
echo "========================================"

# 并发阶梯
CONCURRITIES=(100 200 300 500 800 1000 1500 2000)

for C in "${CONCURRITIES[@]}"; do
  echo ""
  echo ">>> $C 并发连接"
  echo "----------------------------------------"
  wrk -t${THREADS} -c${C} -d${DURATION} --latency "$URL"
  echo ""
done

echo "========================================"
echo "压测完成"
echo "========================================"
