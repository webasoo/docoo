#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ASSETS_DIR="${SCRIPT_DIR}/../assets"
VERSION="${REDOC_VERSION:-2.1.3}"
BASE_URL="https://cdn.jsdelivr.net/npm/redoc@${VERSION}/bundles"

rm -rf "${ASSETS_DIR}"
mkdir -p "${ASSETS_DIR}"

curl -fsSL "${BASE_URL}/redoc.standalone.js" -o "${ASSETS_DIR}/redoc.standalone.js"

cat <<'HTML' > "${ASSETS_DIR}/index.html"
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>Redoc</title>
  <style>
    body { margin: 0; padding: 0; }
    redoc { height: 100vh; }
  </style>
</head>
<body>
  <redoc spec-url="/redoc/openapi.json"></redoc>
  <script src="/redoc/redoc.standalone.js"></script>
</body>
</html>
HTML
