#!/usr/bin/env bash
set -euo pipefail

if [ ! -f .env ]; then
  cp .env.example .env
  echo "Created .env from .env.example. Edit ADMIN_PASSWORD if you want login protection."
fi

mkdir -p data/files data/db data/tmp data/thumbnails data/chroma data/ollama
docker compose up -d --build
echo "MemoDrive is starting: frontend http://localhost:3000, backend http://localhost:8080/api/health"
