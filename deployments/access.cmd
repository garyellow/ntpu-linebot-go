@echo off
setlocal
pushd %~dp0

if "%1"=="up" (
    docker compose -f access/docker-compose.yml up -d
    echo Access gateway started.
    echo Grafana: http://localhost:3000
    echo Prometheus: http://localhost:9090
    echo Alertmanager: http://localhost:9093
    popd
    goto :eof
)

if "%1"=="down" (
    docker compose -f access/docker-compose.yml down
    echo Access gateway stopped.
    popd
    goto :eof
)

echo Usage: access.cmd [up^|down]
popd
exit /b 1
