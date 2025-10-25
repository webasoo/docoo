package scalar

import "embed"

//go:generate bash tools/build.sh

// assets contains a self-hosted Scalar API Reference bundle.
//
//go:embed assets/*
var assets embed.FS
