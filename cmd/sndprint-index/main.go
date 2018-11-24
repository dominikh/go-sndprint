// Command sndprint-index indexes audio files.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"honnef.co/go/sndprint"

	"github.com/jackc/pgx"
)

func parseMBID(mbid string) [16]byte {
	if len(mbid) != 36 {
		return [16]byte{}
	}
	uuid, err := hex.DecodeString(string(mbid[0:8] + mbid[9:13] + mbid[14:18] + mbid[19:23] + mbid[24:36]))
	if err != nil {
		// XXX
		panic(err)
	}
	var out [16]byte
	copy(out[:], uuid)
	return out
}

func main() {
	uuid := flag.String("u", "", "UUID")
	file := flag.String("f", "", "File")
	flag.Parse()
	if *uuid == "" || *file == "" {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-index -u <uuid> -f <file>")
		os.Exit(2)
	}

	conf, err := pgx.ParseEnvLibpq()
	if err != nil {
		panic(err)
	}
	db, err := pgx.Connect(conf)
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

	t := time.Now()
	var data [][]interface{}
	for i := range h1[0] {
		data = append(data, []interface{}{
			int32(h1[0][i]),
			int32(h1[1][i]),
			int32(h1[2][i]),
			int32(h1[3][i]),
			i,
			parseMBID(*uuid),
		})
	}

	n, err := db.CopyFrom(pgx.Identifier{"hashes"}, []string{"hash0", "hash1", "hash2", "hash3", "off", "song"}, pgx.CopyFromRows(data))
	if err != nil {
		panic(err)
	}
	fmt.Println(n)
	fmt.Println(time.Since(t))
}
