#!/usr/bin/env bash
# Runs a local Postgres container for backend dev + Cypress E2E tests.
#
# Usage:
#   ./scripts/dev-postgres.sh          # start (creating the container on first run)
#   ./scripts/dev-postgres.sh --reset  # wipe the container + its data and recreate it
#
# Then export the printed POSTGRES_URL before running the backend or Cypress:
#   eval "$(./scripts/dev-postgres.sh | tail -1)"
set -euo pipefail

CONTAINER_NAME="dopad-postgres"
POSTGRES_USER="dopad"
POSTGRES_PASSWORD="dopad"
POSTGRES_DB="dopad"
POSTGRES_PORT="5432"
POSTGRES_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"

if [[ "${1:-}" == "--reset" ]]; then
  echo "Removing existing ${CONTAINER_NAME} container and its data..." >&2
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
fi

if docker ps --format '{{.Names}}' | grep -qx "${CONTAINER_NAME}"; then
  echo "${CONTAINER_NAME} is already running." >&2
elif docker ps -a --format '{{.Names}}' | grep -qx "${CONTAINER_NAME}"; then
  echo "Starting existing ${CONTAINER_NAME} container..." >&2
  docker start "${CONTAINER_NAME}" >/dev/null
else
  echo "Creating ${CONTAINER_NAME} container..." >&2
  docker run -d \
    --name "${CONTAINER_NAME}" \
    -e POSTGRES_USER="${POSTGRES_USER}" \
    -e POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
    -e POSTGRES_DB="${POSTGRES_DB}" \
    -p "${POSTGRES_PORT}:5432" \
    postgres:16-alpine >/dev/null
fi

echo "Waiting for Postgres to accept connections..." >&2
until docker exec "${CONTAINER_NAME}" pg_isready -U "${POSTGRES_USER}" >/dev/null 2>&1; do
  sleep 1
done
echo "Postgres is ready." >&2

echo "export POSTGRES_URL=\"${POSTGRES_URL}\""
