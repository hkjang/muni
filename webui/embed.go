package webui

import "embed"

// Dist contains the production React bundle. Docker builds replace the placeholder.
//
//go:embed dist/*
var Dist embed.FS
