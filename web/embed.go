package web

import "embed"

//go:embed index.html css/* js/* assets/* partials/*
var Files embed.FS
