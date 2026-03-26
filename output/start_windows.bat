@echo off
chcp 65001 >nul

echo ===============================================
echo UniMAP Multi-Engine Search Service
echo ===============================================
echo.

REM 设置环境变量（可选）
REM set QUAKE_API_KEY=your_quake_api_key
REM set ZOOMEYE_API_KEY=your_zoomeye_api_key
REM set HUNTER_API_KEY=your_hunter_api_key
REM set FOFA_API_KEY=your_fofa_api_key
REM set FOFA_EMAIL=your_fofa_email

echo Starting UniMAP service...
echo.

REM 启动服务
unimap-windows-amd64.exe

echo.
echo Service stopped.
pause