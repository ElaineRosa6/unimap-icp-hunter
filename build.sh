#!/bin/bash

# UniMap Light Cross-Platform Build Script
# 跨平台编译脚本

VERSION="1.0.0"
APP_NAME="unimap-gui"
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
build() {
    local os=$1
    local arch=$2
    local ext=$3
    local output="${OUTPUT_DIR}/${APP_NAME}-${os}-${arch}${ext}"
    
    echo -n "Building for ${os}/${arch}... "
    
    GOOS=${os} GOARCH=${arch} go build -o ${output} ./cmd/unimap-gui
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Success${NC}"
        
        # 获取文件大小
        if [ -f "${output}" ]; then
            size=$(du -h "${output}" | cut -f1)
            echo "  → Output: ${output} (${size})"
        fi
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
echo "  1) All platforms (Windows, macOS, Linux)"
echo "  2) Windows only"
echo "  3) macOS only"
echo "  4) Linux only"
echo "  5) Current platform only"
echo ""
read -p "Enter your choice (1-5): " choice

case $choice in
    1)
        echo -e "\n${YELLOW}Building for all platforms...${NC}\n"
        build "windows" "amd64" ".exe"
        build "windows" "386" ".exe"
        build "darwin" "amd64" ""
        build "darwin" "arm64" ""
        build "linux" "amd64" ""
        build "linux" "386" ""
        build "linux" "arm64" ""
        ;;
    2)
        echo -e "\n${YELLOW}Building for Windows...${NC}\n"
        build "windows" "amd64" ".exe"
        build "windows" "386" ".exe"
        ;;
    3)
        echo -e "\n${YELLOW}Building for macOS...${NC}\n"
        build "darwin" "amd64" ""
        build "darwin" "arm64" ""
        ;;
    4)
        echo -e "\n${YELLOW}Building for Linux...${NC}\n"
        build "linux" "amd64" ""
        build "linux" "386" ""
        build "linux" "arm64" ""
        ;;
    5)
        echo -e "\n${YELLOW}Building for current platform...${NC}\n"
        go build -o ${OUTPUT_DIR}/${APP_NAME} ./cmd/unimap-gui
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ Success${NC}"
            size=$(du -h "${OUTPUT_DIR}/${APP_NAME}" | cut -f1)
            echo "  → Output: ${OUTPUT_DIR}/${APP_NAME} (${size})"
        else
            echo -e "${RED}✗ Failed${NC}"
        fi
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
echo "Usage:"
echo "  Windows: ${APP_NAME}-windows-amd64.exe"
echo "  macOS:   ./${APP_NAME}-darwin-amd64"
echo "  Linux:   ./${APP_NAME}-linux-amd64"
echo ""
