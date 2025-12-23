@echo off
REM Setup script for Monitoring-Only deployment
REM Generates prometheus.yml from template with actual values from .env

setlocal enabledelayedexpansion

pushd %~dp0

REM Check if .env exists
if not exist ".env" (
    echo Error: .env file not found. Please copy .env.example to .env and configure it.
    popd
    exit /b 1
)

REM Load .env file
for /f "usebackq tokens=1,* delims==" %%a in (".env") do (
    set "%%a=%%b"
)

REM Validate required variables
if "%BOT_HOST%"=="" (
    echo Error: BOT_HOST is not set in .env
    popd
    exit /b 1
)

if "%METRICS_PASSWORD%"=="" (
    echo Error: METRICS_PASSWORD is not set in .env
    popd
    exit /b 1
)

REM Set defaults
if "%METRICS_USERNAME%"=="" set "METRICS_USERNAME=prometheus"

REM Generate prometheus.yml from template
echo Generating prometheus.yml...

REM Use PowerShell for sed-like replacement
powershell -Command ^
    "(Get-Content 'prometheus\prometheus.yml.template') ^
    -replace 'BOT_HOST_PLACEHOLDER', '%BOT_HOST%' ^
    -replace 'METRICS_USERNAME_PLACEHOLDER', '%METRICS_USERNAME%' ^
    -replace 'METRICS_PASSWORD_PLACEHOLDER', '%METRICS_PASSWORD%' ^
    | Set-Content 'prometheus\prometheus.yml'"

echo prometheus.yml generated successfully.
echo.
echo Configuration:
echo   Bot Host: %BOT_HOST%
echo   Metrics Username: %METRICS_USERNAME%
echo   Metrics Password: ****
echo.
echo You can now run: docker compose up -d

popd
