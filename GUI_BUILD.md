# UniMap GUI 编译与启动（跨平台）

本项目默认构建不包含 GUI。

- CLI/库（默认）：`go build ./...`
- GUI（Fyne）：需要 `-tags gui`

## 快速开始

### 1) 仅验证/构建（不含 GUI）

```bash
go vet ./...
go test ./...
go build ./...
```

### 2) 启动 GUI（通用命令）

```bash
go run -tags gui ./cmd/unimap-gui
```

或构建可执行文件：

```bash
go build -tags gui -o unimap-gui ./cmd/unimap-gui
```

## GUI 依赖说明

GUI 使用 Fyne(v2) + OpenGL（通过 cgo/glfw）。因此各平台需要可用的 C/C++ 工具链与系统图形相关开发依赖。

### Windows

推荐使用 **MSYS2 MinGW-w64** 提供 gcc 工具链。

1) 安装 MSYS2：https://www.msys2.org/
2) 打开 “MSYS2 MinGW x64” 终端，安装工具链：

```bash
pacman -Syu
pacman -S --needed mingw-w64-x86_64-toolchain
```

3) 确保 `C:\msys64\mingw64\bin` 在 `PATH` 前面（在 PowerShell 里也可以临时设置）：

```powershell
$env:Path = "C:\msys64\mingw64\bin;" + $env:Path
```

4) 构建/运行：

```bash
go build -tags gui ./cmd/unimap-gui
```

字体（中文显示）：
- 默认会尝试加载系统字体（如 `simhei.ttf` / `Deng.ttf`）。
- GUI 主题已对普通文本与 Monospace 文本统一启用 CJK 回退，避免标题/表格在部分机器上出现乱码。
- 也可以手动指定字体文件：

```powershell
$env:UNIMAP_GUI_FONT = "C:\Windows\Fonts\simhei.ttf"
```

字体排查建议：
1) 先确认文件存在：`Test-Path "C:\Windows\Fonts\simhei.ttf"`
2) 再设置环境变量并启动：

```powershell
$env:UNIMAP_GUI_FONT = "C:\Windows\Fonts\simhei.ttf"
go run -tags gui ./cmd/unimap-gui
```

3) 若仍乱码，改用 `Deng.ttf` 或 `Dengl.ttf` 重试。

### Linux

需要 X11/OpenGL 相关开发包（不同发行版包名略有不同）。

Debian/Ubuntu：

```bash
sudo apt-get update
sudo apt-get install -y gcc pkg-config libgl1-mesa-dev xorg-dev
```

Fedora：

```bash
sudo dnf install -y gcc pkgconf-pkg-config mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel
```

Arch：

```bash
sudo pacman -S --needed base-devel pkgconf mesa libxcursor libxrandr libxinerama libxi
```

字体（可选）：如需要更稳的中文显示，可安装 Noto CJK 字体，并通过环境变量指定字体文件路径：

```bash
export UNIMAP_GUI_FONT=/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc
```

### macOS

1) 安装 Xcode Command Line Tools：

```bash
xcode-select --install
```

2) 构建/运行：

```bash
go run -tags gui ./cmd/unimap-gui
```

字体（可选）：如遇中文显示问题，可下载并指定一个支持 CJK 的 TTF/TTC 字体文件：

```bash
export UNIMAP_GUI_FONT="$HOME/Library/Fonts/YourCJKFont.ttf"
```

## 常见问题

- 默认 `go test ./...` 不会构建 GUI：GUI 入口使用 build tag 隔离，避免在缺少 OpenGL/cgo 环境的机器上导致测试失败。
- 如果 `-tags gui` 构建失败且日志里出现 cgo/gcc 相关错误：优先检查是否安装了 gcc/clang 工具链，以及 `PATH` 是否指向正确的 MinGW/编译器。
