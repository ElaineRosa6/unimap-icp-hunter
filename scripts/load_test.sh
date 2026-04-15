#!/usr/bin/env bash
#
# load_test.sh - UniMap 生产负载测试脚本
#
# 用法:
#   ./scripts/load_test.sh [BASE_URL] [CONCURRENCY] [DURATION]
#
# 示例:
#   ./scripts/load_test.sh                        # 默认: http://localhost:8448, 10并发, 30秒
#   ./scripts/load_test.sh http://10.0.0.1:8448   # 指定目标
#   ./scripts/load_test.sh http://localhost:8448 20 60  # 20并发, 60秒
#
# 依赖: curl, jq (可选), 纯 bash/curl 实现无需外部工具
#

set -euo pipefail

# --- 配置 ---
BASE_URL="${1:-http://localhost:8448}"
CONCURRENCY="${2:-10}"
DURATION="${3:-30}"

# 结果目录
RESULTS_DIR="./results/load_test_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[LOAD]${NC} $*"; }
ok()  { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }

# --- 前置检查 ---
log "Checking prerequisites..."
if ! command -v curl &>/dev/null; then
    fail "curl is required but not installed"
    exit 1
fi

# 检查目标是否可达
if ! curl -sf --max-time 5 "$BASE_URL/health" >/dev/null 2>&1; then
    warn "Target $BASE_URL is not reachable or not healthy"
    warn "Make sure UniMap is running: UNIMAP_CONFIG_FILE=configs/unimap.yaml ./unimap-web"
    exit 1
fi
ok "Target $BASE_URL is healthy"

# --- 测试参数 ---
echo ""
log "============================================"
log " UniMap Load Test"
log "============================================"
log " Target:      $BASE_URL"
log " Concurrency: $CONCURRENCY"
log " Duration:    ${DURATION}s per endpoint"
log " Results:     $RESULTS_DIR"
log "============================================"
echo ""

# --- 工具函数 ---
TOTAL_REQUESTS=0
TOTAL_SUCCESS=0
TOTAL_FAILURES=0
TOTAL_429=0

run_load_test() {
    local name="$1"
    local method="$2"
    local path="$3"
    local body_file="$4"
    local content_type="${5:-application/json}"

    log "--- Testing: $name ---"

    local success=0
    local fail=0
    local rate_limited=0
    local p50=0
    local p99=0
    local slowest=0

    local end_time=$(( $(date +%s) + DURATION ))
    local latencies=()

    # 后台运行并发请求
    local pids=()
    local results_file="$RESULTS_DIR/${name}.results"
    > "$results_file"

    for ((i=0; i<CONCURRENCY; i++)); do
        (
            while [ "$(date +%s)" -lt "$end_time" ]; do
                local start_ns=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time()*1e9))' 2>/dev/null || echo "0")
                local http_code
                if [ -n "$body_file" ] && [ -f "$body_file" ]; then
                    http_code=$(curl -s -o /dev/null -w "%{http_code}" \
                        --max-time 10 \
                        -X "$method" \
                        -H "Content-Type: $content_type" \
                        -d @"$body_file" \
                        "$BASE_URL$path" 2>/dev/null || echo "000")
                else
                    http_code=$(curl -s -o /dev/null -w "%{http_code}" \
                        --max-time 10 \
                        -X "$method" \
                        "$BASE_URL$path" 2>/dev/null || echo "000")
                fi
                local end_ns=$(date +%s%N 2>/dev/null || python3 -c 'import time; print(int(time.time()*1e9))' 2>/dev/null || echo "0")

                # 计算延迟（毫秒）
                local latency_ms=0
                if [ "$start_ns" != "0" ] && [ "$end_ns" != "0" ]; then
                    latency_ms=$(( (end_ns - start_ns) / 1000000 ))
                fi

                echo "$http_code $latency_ms" >> "$results_file"
                sleep 0.1  # 每请求间隔 100ms，避免过快
            done
        ) &
        pids+=($!)
    done

    # 等待所有并发完成
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    # 分析结果
    if [ -s "$results_file" ]; then
        local total
        total=$(wc -l < "$results_file" | tr -d ' ')

        success=$(awk '$1 >= 200 && $1 < 300 {count++} END {print count+0}' "$results_file")
        fail=$(awk '$1 >= 400 && $1 != 429 {count++} END {print count+0}' "$results_file")
        rate_limited=$(awk '$1 == 429 {count++} END {print count+0}' "$results_file")

        # 计算延迟统计
        p50=$(awk '{print $2}' "$results_file" | sort -n | awk -v total="$total" 'NR==int(total*0.5)+1 {print $1}')
        p99=$(awk '{print $2}' "$results_file" | sort -n | awk -v total="$total" 'NR==int(total*0.99)+1 {print $1}')
        slowest=$(awk 'BEGIN{max=0} {if($2>max) max=$2} END{print max+0}' "$results_file")

        local rps=0
        if [ "$DURATION" -gt 0 ]; then
            rps=$(( total / DURATION ))
        fi

        echo -e "  ${GREEN}Total:${NC}      $total requests"
        echo -e "  ${GREEN}Success:${NC}    $success ($(( success * 100 / (total > 0 ? total : 1) ))%)"
        echo -e "  ${RED}Errors:${NC}      $fail"
        echo -e "  ${YELLOW}Rate Limited:${NC} $rate_limited"
        echo -e "  ${BLUE}RPS:${NC}        ~$rps"
        echo -e "  ${BLUE}Latency:${NC}    p50=${p50}ms  p99=${p99}ms  max=${slowest}ms"

        TOTAL_REQUESTS=$((TOTAL_REQUESTS + total))
        TOTAL_SUCCESS=$((TOTAL_SUCCESS + success))
        TOTAL_FAILURES=$((TOTAL_FAILURES + fail))
        TOTAL_429=$((TOTAL_429 + rate_limited))
    else
        warn "  No results recorded for $name"
    fi
    echo ""
}

# --- 创建测试 body 文件 ---
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# 查询测试 body
cat > "$TEMP_DIR/query.json" << 'EOF'
{"query":"port:80","engines":["fofa","hunter"],"type":"ip"}
EOF

# 截图测试 body
cat > "$TEMP_DIR/screenshot.json" << 'EOF'
{"url":"https://example.com","engine":"cdp"}
EOF

# 篡改检测测试 body
cat > "$TEMP_DIR/tamper.json" << 'EOF'
{"url":"https://example.com"}
EOF

# 导入 URL 测试 body
cat > "$TEMP_DIR/import_urls.json" << 'EOF'
{"urls":["https://example.com","https://example.org"]}
EOF

# --- 执行测试 ---

# 1. 健康检查（基线）
log "=== Phase 1: Health Check (Baseline) ==="
run_load_test "health" "GET" "/health" ""

# 2. 查询接口
log "=== Phase 2: Query API ==="
run_load_test "query" "GET" "/query?query=port%3A80&engines=fofa&type=ip" ""

# 3. 截图接口（如果服务支持）
log "=== Phase 3: Screenshot API ==="
run_load_test "screenshot" "POST" "/api/screenshot" "$TEMP_DIR/screenshot.json"

# 4. 篡改检测接口
log "=== Phase 4: Tamper Check API ==="
run_load_test "tamper" "POST" "/api/tamper/check" "$TEMP_DIR/tamper.json"

# 5. 导入 URL 接口
log "=== Phase 5: Import URLs API ==="
run_load_test "import_urls" "POST" "/api/import/urls" "$TEMP_DIR/import_urls.json"

# 6. 就绪检查
log "=== Phase 6: Readiness Check ==="
run_load_test "health_ready" "GET" "/health/ready" ""

# --- 汇总 ---
echo ""
log "============================================"
log " Summary"
log "============================================"
log " Total Requests:   $TOTAL_REQUESTS"
log " Successful:       $TOTAL_SUCCESS"
log " Failed:           $TOTAL_FAILURES"
log " Rate Limited:     $TOTAL_429"

if [ "$TOTAL_REQUESTS" -gt 0 ]; then
    success_rate=$(( TOTAL_SUCCESS * 100 / TOTAL_REQUESTS ))
    fail_rate=$(( TOTAL_FAILURES * 100 / TOTAL_REQUESTS ))
    log " Success Rate:     ${success_rate}%"
    log " Failure Rate:     ${fail_rate}%"
fi

if [ "$TOTAL_FAILURES" -gt 0 ]; then
    warn "There were $TOTAL_FAILURES failed requests. Check results for details."
fi

if [ "$TOTAL_429" -gt 0 ]; then
    log "Rate limiting triggered $TOTAL_429 times (expected under load)."
fi

log "============================================"
log " Results saved to: $RESULTS_DIR"
log "============================================"

# 检查是否有 jq 可用，如果有则生成 JSON 报告
if command -v jq &>/dev/null; then
    report_file="$RESULTS_DIR/report.json"
    cat > "$report_file" << EOF
{
  "target": "$BASE_URL",
  "concurrency": $CONCURRENCY,
  "duration_seconds": $DURATION,
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "summary": {
    "total_requests": $TOTAL_REQUESTS,
    "successful": $TOTAL_SUCCESS,
    "failed": $TOTAL_FAILURES,
    "rate_limited": $TOTAL_429
  }
}
EOF
    ok "JSON report: $report_file"
fi

# 退出码：有非 429 错误则返回 1
if [ "$TOTAL_FAILURES" -gt 0 ]; then
    exit 1
fi
exit 0
