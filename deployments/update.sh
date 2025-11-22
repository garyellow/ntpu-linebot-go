#!/bin/bash
# Update all services to latest images
cd "$(dirname "$0")"
docker compose up -d --pull always
# Uncomment to clean up unused images:
# docker image prune -f
