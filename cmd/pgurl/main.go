package main

import (
	"fmt"
	"io"
	"os"

	"http-api-template/config"
)

func run(stdout, stderr io.Writer) int {
	dsn, err := config.LoadMigrationURL()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, dsn); err != nil {
		_, _ = fmt.Fprintln(stderr, "cannot write migration URL")
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr))
}
