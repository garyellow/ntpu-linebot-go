@echo off
REM Update all services to latest images
docker compose up -d --pull always
REM Uncomment to clean up unused images:
REM docker image prune -f
