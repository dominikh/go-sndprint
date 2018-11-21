package main

import (
	"database/sql"
	"fmt"
	"math/bits"
	"os"

	"honnef.co/go/sndprint"
	"honnef.co/go/spew"

	"github.com/lib/pq"
)

func ber(s1, s2 []uint32) float64 {
	e := 0
	for i := range s1 {
		e += bits.OnesCount32(s1[i] ^ s2[i])
	}
	return float64(e) / (256 * 32)
}

const berCutoff = 0.25

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-query <file>")
		os.Exit(2)
	}

	db, err := sql.Open("postgres", "")
	if err != nil {
		panic(err)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not open file:", err)
		os.Exit(2)
	}
	defer f.Close()
	h := sndprint.Hash(f)
	if len(h) < 256 {
		fmt.Fprintln(os.Stderr, "Sample too short")
		os.Exit(2)
	}
	h = h[:256]

	type checkKey struct {
		song     string
		offStart int64
		offEnd   int64
	}
	checked := map[checkKey]float64{}
	for i, hash := range h {
		rows, err := db.Query(`SELECT song, array_agg(off) FROM hashes WHERE hash = $1 GROUP BY song`, int32(hash))
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var song []byte
			var offs []int64
			if err := rows.Scan(&song, pq.Array(&offs)); err != nil {
				panic(err)
			}

			// OPT(dh): we can thin out the result set by coalescing sequential matches for the same song
			for _, off := range offs {
				if int64(i) > off {
					// Impossible match, sample_n can't ever occur before song_n
					continue
				}

				{
					start, end := off-int64(i), off+256-int64(i)
					if _, ok := checked[checkKey{string(song), start, end}]; ok {
						// We've already checked this segment
						continue
					}
					rows, err := db.Query(`SELECT hash FROM hashes WHERE song = $1 AND off >= $2 AND off <= $3 LIMIT 256`, song, off-int64(i), off+256-int64(i))
					if err != nil {
						panic(err)
					}
					hashes := make([]uint32, 0, 256)
					for rows.Next() {
						var hash int32
						if err := rows.Scan(&hash); err != nil {
							panic(err)
						}
						hashes = append(hashes, uint32(hash))
					}
					if rows.Err() != nil {
						panic(rows.Err())
					}
					if len(hashes) != 256 {
						// Too short
						continue
					}

					checked[checkKey{string(song), start, end}] = ber(h, hashes)
				}
			}
		}
		if rows.Err() != nil {
			panic(rows.Err())
		}
	}
	spew.Dump(checked)
}
