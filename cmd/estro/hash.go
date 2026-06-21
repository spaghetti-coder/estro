package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/spaghetti-coder/estro/internal/auth"
)

func runHash(args []string) int {
	var plainPassword string
	switch len(args) {
	case 0:
		fmt.Fprint(os.Stderr, "Password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading password: %v\n", err)
			return 1
		}
		plainPassword = string(pw)
	case 1:
		plainPassword = args[0]
	default:
		fmt.Fprintf(os.Stderr, "usage: %s hash [password]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "  With no argument, prompts for password (no echo).")
		return 1
	}

	hash, err := auth.HashPassword(plainPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(hash)
	return 0
}
