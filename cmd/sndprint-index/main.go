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

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	for off := range h1[0] {
		_, err := tx.Exec(`INSERT INTO hashes (hash0, hash1, hash2, hash3, song, off) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
			int32(h1[0][off]), int32(h1[1][off]), int32(h1[2][off]), int32(h1[3][off]), *uuid, off+16)
		if err != nil {
			panic(err)
		}
	}
	tx.Commit()
}
