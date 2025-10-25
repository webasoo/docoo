#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ASSETS_DIR="${SCRIPT_DIR}/../assets"
VERSION="${SCALAR_VERSION:-1.38.1}"
BASE_URL="https://cdn.jsdelivr.net/npm/@scalar/api-reference@${VERSION}/dist/browser"

rm -rf "${ASSETS_DIR}"
mkdir -p "${ASSETS_DIR}"

curl -fsSL "${BASE_URL}/standalone.js" -o "${ASSETS_DIR}/scalar.min.js"

cat <<'HTML' > "${ASSETS_DIR}/index.html"
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>Scalar API Reference</title>
  <style>
    html, body { height: 100%; margin: 0; }
    #scalar-reference { height: 100%; }
  </style>
</head>
<body>
  <div id="scalar-reference"></div>
  <script src="/scalar/scalar.min.js"></script>
  <script>
    (function () {
      var instance = null;
      function create() {
        if (!window.Scalar || typeof window.Scalar.createApiReference !== 'function') {
          return setTimeout(create, 50);
        }
        instance = window.Scalar.createApiReference('#scalar-reference', {
          url: '/scalar/openapi.json'
        });
      }
      create();
      if (window.addEventListener) {
        window.addEventListener('beforeunload', function () {
          if (instance && typeof instance.destroy === 'function') {
            instance.destroy();
          }
        });
      }
    })();
  </script>
</body>
</html>
HTML

echo "Scalar assets refreshed (version ${VERSION})."
