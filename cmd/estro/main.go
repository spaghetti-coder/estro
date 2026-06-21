package main

import "os"

func main() {
	os.Exit(run(os.Args))
}

func run(args []string) int {
	if len(args) > 1 && args[1] == "hash" {
		return runHash(args[2:])
	}
	return runServer()
}
