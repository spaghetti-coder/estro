package main

import (
	"flag"
	"os"
)

type serverFlags struct {
	configPath string
	staticDir  string
}

func parseServerFlags() serverFlags {
	f := serverFlags{}
	flag.StringVar(&f.configPath, "config", os.Getenv("ESTRO_CONFIG"),
		"path to config file (or set ESTRO_CONFIG env var)")
	flag.StringVar(&f.staticDir, "static-dir", "",
		"path to static files directory")
	flag.Parse()
	return f
}
