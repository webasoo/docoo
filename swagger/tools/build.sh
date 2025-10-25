#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ASSETS_DIR="${SCRIPT_DIR}/../assets"
VERSION="${SWAGGER_UI_VERSION:-5.11.0}"
BASE_URL="https://unpkg.com/swagger-ui-dist@${VERSION}"

rm -rf "${ASSETS_DIR}"
mkdir -p "${ASSETS_DIR}"

curl -fsSL "${BASE_URL}/swagger-ui.css" -o "${ASSETS_DIR}/swagger-ui.css"
curl -fsSL "${BASE_URL}/swagger-ui-bundle.js" -o "${ASSETS_DIR}/swagger-ui-bundle.js"
curl -fsSL "${BASE_URL}/swagger-ui-standalone-preset.js" -o "${ASSETS_DIR}/swagger-ui-standalone-preset.js"
curl -fsSL "${BASE_URL}/favicon-16x16.png" -o "${ASSETS_DIR}/favicon-16x16.png"
curl -fsSL "${BASE_URL}/favicon-32x32.png" -o "${ASSETS_DIR}/favicon-32x32.png"

cat <<'HTML' > "${ASSETS_DIR}/index.html"
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="/swagger/swagger-ui.css" />
  <link rel="icon" type="image/png" href="/swagger/favicon-32x32.png" sizes="32x32" />
  <link rel="icon" type="image/png" href="/swagger/favicon-16x16.png" sizes="16x16" />
  <style>
    body { margin: 0; background: #fafafa; }
  </style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="/swagger/swagger-ui-bundle.js" crossorigin></script>
<script src="/swagger/swagger-ui-standalone-preset.js" crossorigin></script>
<script>
  window.onload = function () {
    const ui = SwaggerUIBundle({
      url: '/swagger/openapi.json',
      dom_id: '#swagger-ui',
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
      layout: 'StandaloneLayout'
    });
    window.ui = ui;
  };
</script>
</body>
</html>
HTML
