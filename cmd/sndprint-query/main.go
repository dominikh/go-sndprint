package main

import (
	"database/sql"
	"fmt"
	"math/bits"
	"os"
	"sort"

	"honnef.co/go/sndprint"
	"honnef.co/go/spew"

	"github.com/lib/pq"
)

func ber(s1, s2 []uint32) float64 {
	e := 0
	for i := range s1 {
		x := uint32(0)
		if i < len(s2) {
			x = s2[i]
		}
		e += bits.OnesCount32(s1[i] ^ x)
	}
	return float64(e) / float64(len(s1)*32)
}

const berCutoff = 0.25

func main() {
	const minSampleLength = 256

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
	if len(h) < minSampleLength {
		fmt.Fprintln(os.Stderr, "Sample too short")
		os.Exit(2)
	}
	sampleLength := len(h)
	type checkKey struct {
		song     string
		offStart int64
		offEnd   int64
	}
	checked := map[checkKey]float64{}
	for i, hash := range h {
		if hash == 0 {
			// A lot of songs have silence
			continue
		}
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
					start, end := off-int64(i), off+int64(sampleLength)-int64(i)
					if _, ok := checked[checkKey{string(song), start, end}]; ok {
						// We've already checked this segment
						continue
					}
					rows, err := db.Query(`SELECT hash FROM hashes WHERE song = $1 AND off >= $2 AND off <= $3 LIMIT $4`, song, start, end, sampleLength)
					if err != nil {
						panic(err)
					}
					hashes := make([]uint32, 0, sampleLength)
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

					checked[checkKey{string(song), start, end}] = ber(h, hashes)
				}
			}
		}
		if rows.Err() != nil {
			panic(rows.Err())
		}
	}

	type result struct {
		key   checkKey
		match float64
	}

	var results []result
	for k, v := range checked {
		results = append(results, result{k, v})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].match < results[j].match
	})
	spew.Dump(results)
}
