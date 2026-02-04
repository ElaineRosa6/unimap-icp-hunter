@echo off
REM UniMap Light Cross-Platform Build Script for Windows
REM 跨平台编译脚本 (Windows版本)

setlocal

set VERSION=1.0.0
set APP_NAME=unimap-gui
set OUTPUT_DIR=dist

echo ========================================
echo UniMap Light Build Script v%VERSION%
echo ========================================
echo.

REM 创建输出目录
if not exist %OUTPUT_DIR% mkdir %OUTPUT_DIR%

REM 检查 Go 是否安装
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo Error: Go is not installed
    echo Please install Go from https://golang.org/dl/
    exit /b 1
)

go version
echo.

echo Select platforms to build:
echo   1^) All platforms ^(Windows, macOS, Linux^)
echo   2^) Windows only
echo   3^) macOS only
echo   4^) Linux only
echo   5^) Current platform only
echo.
set /p choice="Enter your choice (1-5): "

if "%choice%"=="1" goto build_all
if "%choice%"=="2" goto build_windows
if "%choice%"=="3" goto build_macos
if "%choice%"=="4" goto build_linux
if "%choice%"=="5" goto build_current
goto invalid

:build_all
echo.
echo Building for all platforms...
echo.
call :build_platform windows amd64 .exe
call :build_platform windows 386 .exe
call :build_platform darwin amd64
call :build_platform darwin arm64
call :build_platform linux amd64
call :build_platform linux 386
call :build_platform linux arm64
goto done

:build_windows
echo.
echo Building for Windows...
echo.
call :build_platform windows amd64 .exe
call :build_platform windows 386 .exe
goto done

:build_macos
echo.
echo Building for macOS...
echo.
call :build_platform darwin amd64
call :build_platform darwin arm64
goto done

:build_linux
echo.
echo Building for Linux...
echo.
call :build_platform linux amd64
call :build_platform linux 386
call :build_platform linux arm64
goto done

:build_current
echo.
echo Building for current platform...
echo.
go build -o %OUTPUT_DIR%\%APP_NAME%.exe .\cmd\unimap-gui
if %errorlevel% equ 0 (
    echo [92m✓ Success[0m
    dir %OUTPUT_DIR%\%APP_NAME%.exe | find "%APP_NAME%.exe"
) else (
    echo [91m✗ Failed[0m
)
goto done

:build_platform
set os=%~1
set arch=%~2
set ext=%~3
set output=%OUTPUT_DIR%\%APP_NAME%-%os%-%arch%%ext%

echo Building for %os%/%arch%...
set GOOS=%os%
set GOARCH=%arch%
go build -o %output% .\cmd\unimap-gui
if %errorlevel% equ 0 (
    echo [92m✓ Success[0m
    echo   -^> Output: %output%
) else (
    echo [91m✗ Failed[0m
)
goto :eof

:invalid
echo Invalid choice
exit /b 1

:done
echo.
echo ========================================
echo Build completed!
echo ========================================
echo.
echo Output directory: %OUTPUT_DIR%\
dir /B %OUTPUT_DIR% 2>nul
if %errorlevel% neq 0 echo No files generated
echo.
echo Usage:
echo   Windows: %APP_NAME%-windows-amd64.exe
echo   macOS:   ./%APP_NAME%-darwin-amd64
echo   Linux:   ./%APP_NAME%-linux-amd64
echo.
pause
