@echo off
echo Stopping any existing server on port 8080...
for /f "tokens=5" %%p in ('netstat -ano ^| findstr ":8080 "') do (
    taskkill /PID %%p /F >nul 2>&1
)

echo Building...
go build -o rplace.exe .

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [!] Build failed.
    pause
    exit /b %ERRORLEVEL%
)

echo Starting server...
echo -----------------------
rplace.exe