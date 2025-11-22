#!/bin/bash
cd "$(dirname "$0")"

if [ "$1" = "up" ]; then
    docker compose -f access/docker-compose.yaml up -d
    echo "Access gateway started."
    echo "Grafana: http://localhost:3000"
    echo "Prometheus: http://localhost:9090"
    echo "Alertmanager: http://localhost:9093"
elif [ "$1" = "down" ]; then
    docker compose -f access/docker-compose.yaml down
    echo "Access gateway stopped."
else
    echo "Usage: ./access.sh [up|down]"
    exit 1
fi
