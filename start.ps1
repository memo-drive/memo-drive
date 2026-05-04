if (-not (Test-Path ".env")) {
  Copy-Item ".env.example" ".env"
  Write-Host "Created .env from .env.example. Edit ADMIN_PASSWORD if you want login protection."
}

New-Item -ItemType Directory -Force -Path "data/files", "data/db", "data/tmp", "data/thumbnails", "data/chroma", "data/ollama" | Out-Null
docker compose up -d --build
Write-Host "MemoDrive is starting: frontend http://localhost:3000, backend http://localhost:8080/api/health"
