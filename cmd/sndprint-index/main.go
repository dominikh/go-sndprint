// Command sndprint-index indexes audio files.
package main

import (
	"flag"
	"fmt"
	"os"

	"honnef.co/go/sndprint"
	"honnef.co/go/sndprint/sndprintdb"
)

func main() {
	uuid := flag.String("u", "", "UUID")
	file := flag.String("f", "", "File")
	flag.Parse()
	if *uuid == "" || *file == "" {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-index -u <uuid> -f <file>")
		os.Exit(2)
	}
	db, err := sndprintdb.Open("/home/dominikh/prj/src/honnef.co/go/sndprint/_fingerprints/")
	if err != nil {
		panic(err) // XXX
	}

	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't open file:", err)
		os.Exit(2)
	}
	defer f.Close()
	h1 := sndprint.Hash(f, 0)
	if err := db.AddSong(*uuid, h1); err != nil {
		panic(err) // XXX
	}
}
