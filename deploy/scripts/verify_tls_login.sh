#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <domain> [password]"
  echo "Example: $0 drive.example.com your-admin-password"
  exit 1
fi

DOMAIN="$1"
PASSWORD="${2:-}"

echo "==> 1) HTTP must redirect to HTTPS"
curl -sS -o /dev/null -w "HTTP status: %{http_code}\nRedirect: %{redirect_url}\n" "http://${DOMAIN}/"

echo
echo "==> 2) TLS handshake and certificate subject"
openssl s_client -connect "${DOMAIN}:443" -servername "${DOMAIN}" </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates

echo
echo "==> 3) HSTS header check"
curl -sSI "https://${DOMAIN}/" | rg -i "strict-transport-security|server:"

if [[ -n "${PASSWORD}" ]]; then
  echo
  echo "==> 4) Login endpoint over HTTPS"
  curl -sS -o /dev/null -w "HTTP status: %{http_code}\n" \
    -H "Content-Type: application/json" \
    -d "{\"password\":\"${PASSWORD}\"}" \
    "https://${DOMAIN}/api/auth/login"
fi

echo
echo "Done. For final check, use browser DevTools Network to confirm /api/auth/login is https:// and no Mixed Content warnings."
