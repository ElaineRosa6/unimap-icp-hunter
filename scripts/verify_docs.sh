#!/bin/bash
# 文档一致性校验脚本
# 检查 README.md 与代码实现的一致性

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

echo "========================================="
echo "文档一致性校验"
echo "========================================="
echo ""

# 1. 检查目录结构
echo -e "${YELLOW}[1] 检查目录结构...${NC}"

check_directory() {
    local dir=$1
    if [ -d "$dir" ]; then
        echo -e "  ${GREEN}✓${NC} $dir 存在"
    else
        echo -e "  ${RED}✗${NC} $dir 不存在"
        ((ERRORS++))
    fi
}

check_directory "cmd/unimap-cli"
check_directory "cmd/unimap-gui"
check_directory "cmd/unimap-web"
check_directory "internal/adapter"
check_directory "internal/core/unimap"
check_directory "internal/service"
check_directory "internal/plugin"
check_directory "internal/screenshot"
check_directory "internal/tamper"
check_directory "internal/config"
check_directory "web"
check_directory "configs"

echo ""

# 2. 检查关键文件
echo -e "${YELLOW}[2] 检查关键文件...${NC}"

check_file() {
    local file=$1
    if [ -f "$file" ]; then
        echo -e "  ${GREEN}✓${NC} $file 存在"
    else
        echo -e "  ${RED}✗${NC} $file 不存在"
        ((ERRORS++))
    fi
}

check_file "web/server.go"
check_file "configs/config.yaml.example"
check_file "QUICKSTART.md"

echo ""

# 3. 检查配置参数一致性
echo -e "${YELLOW}[3] 检查配置参数...${NC}"

# 检查 config.go 中是否包含 README 提到的配置项
CONFIG_FILE="internal/config/config.go"
if [ -f "$CONFIG_FILE" ]; then
    # 检查 max_concurrent
    if grep -q "MaxConcurrent" "$CONFIG_FILE"; then
        echo -e "  ${GREEN}✓${NC} MaxConcurrent 配置项存在"
    else
        echo -e "  ${RED}✗${NC} MaxConcurrent 配置项不存在"
        ((ERRORS++))
    fi

    # 检查 cache_ttl
    if grep -q "CacheTTL" "$CONFIG_FILE"; then
        echo -e "  ${GREEN}✓${NC} CacheTTL 配置项存在"
    else
        echo -e "  ${RED}✗${NC} CacheTTL 配置项不存在"
        ((ERRORS++))
    fi

    # 检查 cache_max_size
    if grep -q "CacheMaxSize" "$CONFIG_FILE"; then
        echo -e "  ${GREEN}✓${NC} CacheMaxSize 配置项存在"
    else
        echo -e "  ${RED}✗${NC} CacheMaxSize 配置项不存在"
        ((ERRORS++))
    fi

    # 检查 cache_cleanup_interval
    if grep -q "CacheCleanupInterval" "$CONFIG_FILE"; then
        echo -e "  ${GREEN}✓${NC} CacheCleanupInterval 配置项存在"
    else
        echo -e "  ${RED}✗${NC} CacheCleanupInterval 配置项不存在"
        ((ERRORS++))
    fi
else
    echo -e "  ${RED}✗${NC} $CONFIG_FILE 不存在"
    ((ERRORS++))
fi

echo ""

# 4. 检查引擎适配器
echo -e "${YELLOW}[4] 检查引擎适配器...${NC}"

ADAPTER_DIR="internal/adapter"
engines=("fofa" "hunter" "zoomeye" "quake" "shodan")

for engine in "${engines[@]}"; do
    if ls "$ADAPTER_DIR"/*.go 2>/dev/null | xargs grep -l -i "$engine" > /dev/null; then
        echo -e "  ${GREEN}✓${NC} $engine 适配器存在"
    else
        echo -e "  ${YELLOW}!${NC} $engine 适配器未找到（可能为外部插件）"
        ((WARNINGS++))
    fi
done

echo ""

# 5. 检查 Go 版本
echo -e "${YELLOW}[5] 检查 Go 版本要求...${NC}"

GO_MOD="go.mod"
if [ -f "$GO_MOD" ]; then
    GO_VERSION=$(grep -E "^go [0-9]" "$GO_MOD" | awk '{print $2}')
    README_VERSION=$(grep "Go 1\.[0-9]" README.md | head -1 | grep -oE "1\.[0-9]+(\.[0-9]+)?")

    if [ "$GO_VERSION" = "$README_VERSION" ]; then
        echo -e "  ${GREEN}✓${NC} Go 版本一致: $GO_VERSION"
    else
        echo -e "  ${YELLOW}!${NC} Go 版本可能不一致: go.mod=$GO_VERSION, README=$README_VERSION"
        ((WARNINGS++))
    fi
else
    echo -e "  ${RED}✗${NC} go.mod 不存在"
    ((ERRORS++))
fi

echo ""

# 6. 检查编译
echo -e "${YELLOW}[6] 检查编译状态...${NC}"

if go build ./... 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} 编译通过"
else
    echo -e "  ${RED}✗${NC} 编译失败"
    ((ERRORS++))
fi

echo ""

# 7. 检查测试
echo -e "${YELLOW}[7] 检查测试状态...${NC}"

if go test ./... 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} 测试通过"
else
    echo -e "  ${RED}✗${NC} 测试失败"
    ((ERRORS++))
fi

echo ""

# 8. 检查相关文档是否存在
echo -e "${YELLOW}[8] 检查相关文档...${NC}"

docs=("QUICKSTART.md" "USAGE.md" "README_LIGHT.md" "PROJECT_SUMMARY.md")

for doc in "${docs[@]}"; do
    if [ -f "$doc" ]; then
        echo -e "  ${GREEN}✓${NC} $doc 存在"
    else
        echo -e "  ${YELLOW}!${NC} $doc 不存在"
        ((WARNINGS++))
    fi
done

echo ""

# 总结
echo "========================================="
echo "校验结果"
echo "========================================="
echo -e "错误: ${RED}$ERRORS${NC}"
echo -e "警告: ${YELLOW}$WARNINGS${NC}"
echo ""

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}发现 $ERRORS 个错误，请检查文档一致性！${NC}"
    exit 1
elif [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}有 $WARNINGS 个警告，建议检查。${NC}"
    exit 0
else
    echo -e "${GREEN}所有检查通过，文档与代码一致。${NC}"
    exit 0
fi