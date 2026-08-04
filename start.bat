@echo off
cd /d "%~dp0"

REM 若网关已在运行:直接打开管理界面,窗口驻留
tasklist /FI "IMAGENAME eq gateway.exe" 2>nul | findstr /I "gateway.exe" >nul
if errorlevel 1 goto :start
echo [LLM GATEWAY] 网关已在运行,打开管理界面 http://localhost:8080 ...
timeout /t 1 /nobreak >nul
start "" http://localhost:8080
pause
exit /b

:start
echo [LLM GATEWAY] 启动网关...
echo [LLM GATEWAY] 管理界面: http://localhost:8080 (仅本机可访问,启动后自动打开浏览器)
echo [LLM GATEWAY] 关闭本窗口或按 Ctrl+C 即可停止网关
echo.
gateway.exe -config config.json
pause
