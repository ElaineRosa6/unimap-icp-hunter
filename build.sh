#!/bin/bash

# UniMap Build Script
# 一键构建脚本：web + cli + gui

VERSION="1.0.0"
OUTPUT_DIR="dist"

echo "========================================"
echo "UniMap Light Build Script v${VERSION}"
echo "========================================"
echo ""

# 创建输出目录
mkdir -p ${OUTPUT_DIR}

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 构建函数
build_component() {
    local component=$1
    local os=$2
    local arch=$3
    local ext=$4
    local output="${OUTPUT_DIR}/${component}-${os}-${arch}${ext}"

    echo -n "Building ${component} for ${os}/${arch}... "

    GOOS=${os} GOARCH=${arch} go build -o "${output}" "./cmd/${component}"

    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Success${NC}"
        if [ -f "${output}" ]; then
            size=$(du -h "${output}" | cut -f1)
            echo "  → Output: ${output} (${size})"
        fi
    else
        echo -e "${RED}✗ Failed${NC}"
    fi
}

build_current() {
    local component=$1
    local output="${OUTPUT_DIR}/${component}"
    echo -n "Building ${component} (current platform)... "
    go build -o "${output}" "./cmd/${component}"
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Success${NC}"
        size=$(du -h "${output}" | cut -f1)
        echo "  → Output: ${output} (${size})"
    else
        echo -e "${RED}✗ Failed${NC}"
    fi
}

# 检查 Go 是否安装
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

echo "Go version: $(go version)"
echo ""

# 询问用户要编译的平台
echo "Select platforms to build:"
echo "  1) All platforms (Web+CLI cross-compile; GUI current only)"
echo "  2) Current platform only (Web+CLI+GUI)"
echo "  3) Web + CLI only (all platforms)"
echo "  4) GUI only (current platform)"
echo ""
read -p "Enter your choice (1-4): " choice

case $choice in
    1)
        echo -e "\n${YELLOW}Building Web+CLI for all platforms...${NC}\n"
        build_component "unimap-web" "windows" "amd64" ".exe"
        build_component "unimap-web" "windows" "386" ".exe"
        build_component "unimap-web" "darwin" "amd64" ""
        build_component "unimap-web" "darwin" "arm64" ""
        build_component "unimap-web" "linux" "amd64" ""
        build_component "unimap-web" "linux" "386" ""
        build_component "unimap-web" "linux" "arm64" ""

        build_component "unimap-cli" "windows" "amd64" ".exe"
        build_component "unimap-cli" "windows" "386" ".exe"
        build_component "unimap-cli" "darwin" "amd64" ""
        build_component "unimap-cli" "darwin" "arm64" ""
        build_component "unimap-cli" "linux" "amd64" ""
        build_component "unimap-cli" "linux" "386" ""
        build_component "unimap-cli" "linux" "arm64" ""

        echo -e "\n${YELLOW}Building GUI for current platform only...${NC}\n"
        build_current "unimap-gui"
        ;;
    2)
        echo -e "\n${YELLOW}Building for current platform (Web+CLI+GUI)...${NC}\n"
        build_current "unimap-web"
        build_current "unimap-cli"
        build_current "unimap-gui"

        echo -e "\n${YELLOW}Copying runtime assets to ${OUTPUT_DIR}/...${NC}\n"
        rm -rf "${OUTPUT_DIR}/web" "${OUTPUT_DIR}/configs"
        mkdir -p "${OUTPUT_DIR}/web"
        if [ -d "web/templates" ]; then
            cp -R "web/templates" "${OUTPUT_DIR}/web/templates"
        fi
        if [ -d "web/static" ]; then
            cp -R "web/static" "${OUTPUT_DIR}/web/static"
        fi
        if [ -d "configs" ]; then
            cp -R "configs" "${OUTPUT_DIR}/configs"
        fi
        ;;
    3)
        echo -e "\n${YELLOW}Building Web+CLI for all platforms...${NC}\n"
        build_component "unimap-web" "windows" "amd64" ".exe"
        build_component "unimap-web" "windows" "386" ".exe"
        build_component "unimap-web" "darwin" "amd64" ""
        build_component "unimap-web" "darwin" "arm64" ""
        build_component "unimap-web" "linux" "amd64" ""
        build_component "unimap-web" "linux" "386" ""
        build_component "unimap-web" "linux" "arm64" ""

        build_component "unimap-cli" "windows" "amd64" ".exe"
        build_component "unimap-cli" "windows" "386" ".exe"
        build_component "unimap-cli" "darwin" "amd64" ""
        build_component "unimap-cli" "darwin" "arm64" ""
        build_component "unimap-cli" "linux" "amd64" ""
        build_component "unimap-cli" "linux" "386" ""
        build_component "unimap-cli" "linux" "arm64" ""
        ;;
    4)
        echo -e "\n${YELLOW}Building GUI for current platform...${NC}\n"
        build_current "unimap-gui"
        ;;
    *)
        echo -e "${RED}Invalid choice${NC}"
        exit 1
        ;;
esac

echo ""
echo "========================================"
echo "Build completed!"
echo "========================================"
echo ""
echo "Output directory: ${OUTPUT_DIR}/"
ls -lh ${OUTPUT_DIR}/ 2>/dev/null || echo "No files generated"
echo ""
echo "Outputs:"
echo "  Web: ${OUTPUT_DIR}/unimap-web*"
echo "  CLI: ${OUTPUT_DIR}/unimap-cli*"
echo "  GUI: ${OUTPUT_DIR}/unimap-gui"
echo ""
