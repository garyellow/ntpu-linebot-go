#!/bin/bash
# Setup script for Monitoring-Only deployment
# Generates prometheus.yml from template with actual values from .env

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Load .env file
if [ ! -f "$SCRIPT_DIR/.env" ]; then
    echo "Error: .env file not found. Please copy .env.example to .env and configure it."
    exit 1
fi

# shellcheck disable=SC1091
source "$SCRIPT_DIR/.env"

# Validate required variables
if [ -z "$BOT_HOST" ]; then
    echo "Error: BOT_HOST is not set in .env"
    exit 1
fi

if [ -z "$METRICS_PASSWORD" ]; then
    echo "Error: METRICS_PASSWORD is not set in .env"
    exit 1
fi

# Set defaults
METRICS_USERNAME="${METRICS_USERNAME:-prometheus}"

# Generate prometheus.yml from template
echo "Generating prometheus.yml..."
sed \
    -e "s|BOT_HOST_PLACEHOLDER|$BOT_HOST|g" \
    -e "s|METRICS_USERNAME_PLACEHOLDER|$METRICS_USERNAME|g" \
    -e "s|METRICS_PASSWORD_PLACEHOLDER|$METRICS_PASSWORD|g" \
    "$SCRIPT_DIR/prometheus/prometheus.yml.template" > "$SCRIPT_DIR/prometheus/prometheus.yml"

echo "prometheus.yml generated successfully."
echo ""
echo "Configuration:"
echo "  Bot Host: $BOT_HOST"
echo "  Metrics Username: $METRICS_USERNAME"
echo "  Metrics Password: ****"
echo ""
echo "You can now run: docker compose up -d"
