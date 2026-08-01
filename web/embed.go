package web

import "embed"

//go:embed index.html css/* js/* assets/*
var Files embed.FS
