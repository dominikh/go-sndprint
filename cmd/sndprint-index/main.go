// Command sndprint-index indexes audio files.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	"honnef.co/go/sndprint"

	_ "github.com/lib/pq"
)

func main() {
	uuid := flag.String("u", "", "UUID")
	file := flag.String("f", "", "File")
	flag.Parse()
	if *uuid == "" || *file == "" {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-index -u <uuid> -f <file>")
		os.Exit(2)
	}

	db, err := sql.Open("postgres", "")
	if err != nil {
		panic(err)
	}

	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't open file:", err)
		os.Exit(2)
	}
	defer f.Close()
	h1 := sndprint.Hash(f)
	for off, h := range h1 {
		_, err := db.Exec(`INSERT INTO hashes (hash, song, off) VALUES ($1, $2, $3)`, int32(h), *uuid, off)
		if err != nil {
			panic(err)
		}
	}
}
