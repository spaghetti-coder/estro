package estro

import "embed"

//go:embed all:public
var StaticFS embed.FS
