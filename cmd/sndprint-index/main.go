// Command sndprint-index indexes audio files.
package main

import (
	"flag"
	"os"

	"honnef.co/go/sndprint"
	"honnef.co/go/sndprint/cmdutil"
)

func main() {
	uuid := flag.String("u", "", "UUID")
	file := flag.String("f", "", "File")
	flag.Parse()
	if *uuid == "" || *file == "" {
		cmdutil.Usage("Usage: sndprint-index -u <uuid> -f <file>")
	}

	db, err := cmdutil.DB()
	if err != nil {
		cmdutil.Die("Could not open fingerprint database:", err)
	}

	f, err := os.Open(*file)
	if err != nil {
		cmdutil.Die("Could not open file:", err)
	}
	defer f.Close()
	h1 := sndprint.Hash(f)
	if err := db.AddSong(*uuid, h1); err != nil {
		cmdutil.Die("Could not index file:", err)
	}
}
