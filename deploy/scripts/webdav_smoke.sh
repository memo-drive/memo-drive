#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEMODRIVE_URL:-http://localhost:8080}"
BASE_URL="${BASE_URL%/}"
MEMODRIVE_USER="${MEMODRIVE_USER:-admin}"
MEMODRIVE_PASSWORD="${MEMODRIVE_PASSWORD:-}"
RUN_ID="${MEMODRIVE_SMOKE_ID:-webdav-smoke-$(date +%s)}"

SOURCE_NAME="${RUN_ID}.txt"
MOVED_NAME="${RUN_ID}-moved.txt"
COPY_NAME="${RUN_ID}-copy.txt"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl_auth=()
if [[ -n "$MEMODRIVE_PASSWORD" ]]; then
  curl_auth=(-u "${MEMODRIVE_USER}:${MEMODRIVE_PASSWORD}")
fi

api_auth=()
if [[ -n "$MEMODRIVE_PASSWORD" ]]; then
  login_body="$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
    -H "Content-Type: application/json" \
    --data "{\"password\":\"${MEMODRIVE_PASSWORD}\"}")"
  token="$(printf '%s' "$login_body" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
  if [[ -z "$token" ]]; then
    echo "failed to parse API login token" >&2
    exit 1
  fi
  api_auth=(-H "Authorization: Bearer ${token}")
fi

source_file="${TMP_DIR}/source.txt"
downloaded_file="${TMP_DIR}/downloaded.txt"
copied_file="${TMP_DIR}/copied.txt"
printf 'MemoDrive WebDAV smoke %s\n' "$RUN_ID" > "$source_file"

echo "PROPFIND /dav/"
curl -X PROPFIND "${curl_auth[@]}" -fsS "${BASE_URL}/dav/" \
  -H "Depth: 0" \
  -H "Content-Type: application/xml" \
  --data-binary '<?xml version="1.0"?><D:propfind xmlns:D="DAV:"><D:prop><D:displayname/><D:resourcetype/><D:quota-used-bytes/></D:prop></D:propfind>' \
  > /dev/null

echo "PUT /dav/${SOURCE_NAME}"
curl -T "$source_file" "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${SOURCE_NAME}" > /dev/null

echo "GET /dav/${SOURCE_NAME}"
curl "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${SOURCE_NAME}" -o "$downloaded_file"
cmp "$source_file" "$downloaded_file"

echo "MOVE /dav/${SOURCE_NAME} -> /dav/${MOVED_NAME}"
curl -X MOVE "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${SOURCE_NAME}" \
  -H "Destination: ${BASE_URL}/dav/${MOVED_NAME}" \
  > /dev/null

echo "COPY /dav/${MOVED_NAME} -> /dav/${COPY_NAME}"
curl -X COPY "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${MOVED_NAME}" \
  -H "Destination: ${BASE_URL}/dav/${COPY_NAME}" \
  > /dev/null

echo "GET /dav/${COPY_NAME}"
curl "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${COPY_NAME}" -o "$copied_file"
cmp "$source_file" "$copied_file"

echo "Check /api/files sees WebDAV upload"
files_body="$(curl "${api_auth[@]}" -fsS "${BASE_URL}/api/files?path=/")"
printf '%s' "$files_body" | grep -F "\"name\":\"${MOVED_NAME}\"" > /dev/null
printf '%s' "$files_body" | grep -E '"status":"(uploaded|processing|ready|failed)"' > /dev/null

echo "DELETE /dav/${COPY_NAME}"
curl -X DELETE "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${COPY_NAME}" > /dev/null

echo "Check /trash sees WebDAV delete"
curl "${api_auth[@]}" -fsS "${BASE_URL}/api/trash?limit=20" | grep -F "\"original_name\":\"${COPY_NAME}\"" > /dev/null

echo "Cleanup /dav/${MOVED_NAME}"
curl -X DELETE "${curl_auth[@]}" -fsS "${BASE_URL}/dav/${MOVED_NAME}" > /dev/null

echo "WebDAV smoke passed for ${BASE_URL}/dav"
