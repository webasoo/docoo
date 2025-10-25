package swagger

import "embed"

//go:generate bash tools/build.sh

// assets contains a vendored subset of swagger-ui-dist plus a custom index.html.
// The files are generated via tools/build.sh and embedded for offline use.
//
//go:embed assets/*
var assets embed.FS
