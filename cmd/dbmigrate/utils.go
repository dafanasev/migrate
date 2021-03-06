package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
)

// exitWithError prints an error to the terminal and terminates app with error
func exitWithError(err error) {
	if migrator != nil {
		migrator.Close()
		migrator = nil
	}

	fmt.Fprintln(os.Stderr, errors.Wrap(err, "can't run dbmigrate"))
	os.Exit(1)
}

// pluralize currently just adds 's' to the given word if n != 1
func pluralize(s string, n int) string {
	if n != 1 {
		return s + "s"
	}
	return s
}
