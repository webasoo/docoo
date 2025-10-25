package redoc

import "embed"

//go:generate bash tools/build.sh

// assets holds a self-contained Redoc distribution for offline use.
//
//go:embed assets/*
var assets embed.FS
