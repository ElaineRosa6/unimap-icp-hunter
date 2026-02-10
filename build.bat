@echo off
REM UniMap Build Script for Windows
REM 一键构建脚本 (Windows版本)：web + cli + gui

setlocal

set VERSION=1.0.0
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
echo   1^) All platforms ^(Web+CLI cross-compile; GUI current only^)
echo   2^) Current platform only ^(Web+CLI+GUI^)
echo   3^) Web + CLI only ^(all platforms^)
echo   4^) GUI only ^(current platform^)
echo.
set /p choice="Enter your choice (1-4): "

if "%choice%"=="1" goto build_all
if "%choice%"=="2" goto build_current
if "%choice%"=="3" goto build_webcli_all
if "%choice%"=="4" goto build_gui_current
goto invalid

:build_all
echo.
echo Building Web+CLI for all platforms...
echo.
call :build_component unimap-web windows amd64 .exe
call :build_component unimap-web windows 386 .exe
call :build_component unimap-web darwin amd64
call :build_component unimap-web darwin arm64
call :build_component unimap-web linux amd64
call :build_component unimap-web linux 386
call :build_component unimap-web linux arm64

call :build_component unimap-cli windows amd64 .exe
call :build_component unimap-cli windows 386 .exe
call :build_component unimap-cli darwin amd64
call :build_component unimap-cli darwin arm64
call :build_component unimap-cli linux amd64
call :build_component unimap-cli linux 386
call :build_component unimap-cli linux arm64

echo.
echo Building GUI for current platform only...
call :build_gui_current
goto done

:build_webcli_all
echo.
echo Building Web+CLI for all platforms...
echo.
call :build_component unimap-web windows amd64 .exe
call :build_component unimap-web windows 386 .exe
call :build_component unimap-web darwin amd64
call :build_component unimap-web darwin arm64
call :build_component unimap-web linux amd64
call :build_component unimap-web linux 386
call :build_component unimap-web linux arm64

call :build_component unimap-cli windows amd64 .exe
call :build_component unimap-cli windows 386 .exe
call :build_component unimap-cli darwin amd64
call :build_component unimap-cli darwin arm64
call :build_component unimap-cli linux amd64
call :build_component unimap-cli linux 386
call :build_component unimap-cli linux arm64
goto done

:build_current
echo.
echo Building for current platform (Web+CLI+GUI)...
echo.
call :build_component_current unimap-web
call :build_component_current unimap-cli
call :build_gui_current

REM Copy runtime assets for Web into dist so it can run standalone
REM (copy only templates/static to avoid putting .go sources under dist/)
if exist %OUTPUT_DIR%\web rmdir /S /Q %OUTPUT_DIR%\web
if exist web\templates (
    if not exist %OUTPUT_DIR%\web\templates mkdir %OUTPUT_DIR%\web\templates
    xcopy /E /I /Y web\templates %OUTPUT_DIR%\web\templates >nul
)
if exist web\static (
    if not exist %OUTPUT_DIR%\web\static mkdir %OUTPUT_DIR%\web\static
    xcopy /E /I /Y web\static %OUTPUT_DIR%\web\static >nul
)
if exist configs (
    if not exist %OUTPUT_DIR%\configs mkdir %OUTPUT_DIR%\configs
    xcopy /E /I /Y configs %OUTPUT_DIR%\configs >nul
)
goto done

:build_gui_current
echo Building GUI (current platform)...
go build -o %OUTPUT_DIR%\unimap-gui.exe .\cmd\unimap-gui
if %errorlevel% equ 0 (
    echo   -> OK: %OUTPUT_DIR%\unimap-gui.exe
) else (
    echo   -> FAILED: GUI build (requires CGO toolchain for Fyne)
)
goto :eof

:build_component
setlocal
set component=%~1
set os=%~2
set arch=%~3
set ext=%~4
set output=%OUTPUT_DIR%\%component%-%os%-%arch%%ext%

echo Building %component% for %os%/%arch%...
set GOOS=%os%
set GOARCH=%arch%
go build -o %output% .\cmd\%component%
if %errorlevel% equ 0 (
    echo   -> OK: %output%
) else (
    echo   -> FAILED: %component% %os%/%arch%
)
endlocal
goto :eof

:build_component_current
set component=%~1
echo Building %component% (current platform)...
go build -o %OUTPUT_DIR%\%component%.exe .\cmd\%component%
if %errorlevel% equ 0 (
    echo   -> OK: %OUTPUT_DIR%\%component%.exe
) else (
    echo   -> FAILED: %component%
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
echo Outputs:
echo   Web: %OUTPUT_DIR%\unimap-web*.exe
echo   CLI: %OUTPUT_DIR%\unimap-cli*.exe
echo   GUI: %OUTPUT_DIR%\unimap-gui.exe
echo.
pause
