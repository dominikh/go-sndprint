package cmdutil

import (
	"errors"
	"fmt"
	"os"

	"honnef.co/go/sndprint/sndprintdb"
)

func DB() (*sndprintdb.DB, error) {
	path := os.Getenv("SNDPRINT_DB")
	if path == "" {
		return nil, errors.New("SNDPRINT_DB not set")
	}
	return sndprintdb.Open(path)
}

func Die(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(2)
}

func Usage(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}
