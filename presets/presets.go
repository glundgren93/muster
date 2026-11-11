package presets

import "embed"

//go:embed */config.toml */README.md */hooks/*
var Files embed.FS
