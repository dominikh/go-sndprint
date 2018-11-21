// Command sndprint-fp generates fingerprints for an audio file.
package main

import (
	"fmt"
	"io"
	"os"

	"honnef.co/go/sndprint"
)

func main() {
	var r io.Reader
	switch len(os.Args) {
	case 1:
		r = os.Stdin
	case 2:
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not open file:", err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	default:
		fmt.Fprintln(os.Stderr, "Usage: sndprint-fp [file]")
		os.Exit(2)
	}
	hashes := sndprint.Hash(r)
	for _, hash := range hashes {
		fmt.Printf("%#08x\n", hash)
	}
}
