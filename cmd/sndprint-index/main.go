// Command sndprint-index indexes audio files.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	"honnef.co/go/sndprint"

	"github.com/lib/pq"
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
	h1 := sndprint.Hash(f, 0)

	var h2 [len(h1)][]int32
	for i := range h1 {
		h2[i] = make([]int32, len(h1[i]))
		for j := range h1[i] {
			h2[i][j] = int32(h1[i][j])
		}
	}

	_, err = db.Exec(`INSERT INTO hashes (hash0, hash1, hash2, hash3, off, song) (SELECT *, $5::UUID FROM UNNEST ($1::integer[], $2::integer[], $3::integer[], $4::integer[]) WITH ORDINALITY) ON CONFLICT DO NOTHING`,
		pq.Array(h2[0]), pq.Array(h2[1]), pq.Array(h2[2]), pq.Array(h2[3]), *uuid)
	if err != nil {
		panic(err)
	}
}
