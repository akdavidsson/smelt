package main

import (
	"fmt"
	"os"

	"github.com/arnordavidsson/smelt/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
