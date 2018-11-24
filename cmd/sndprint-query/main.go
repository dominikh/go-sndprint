package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"math/bits"
	"os"
	"sort"

	"github.com/lib/pq"
	"honnef.co/go/sndprint"
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
	verbose := flag.Bool("v", false, "Enable verbose output")
	seconds := flag.Int("t", 0, "Max seconds to process")
	flag.Parse()

	const minSampleLength = 256

	if len(flag.Args()) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-query [file]")
		os.Exit(2)
	}

	db, err := sql.Open("postgres", "")
	if err != nil {
		panic(err)
	}

	var r io.Reader = os.Stdin
	if len(flag.Args()) == 2 {
		f, err := os.Open(flag.Args()[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not open file:", err)
			os.Exit(2)
		}
		defer f.Close()
		r = f
	}

	h := sndprint.Hash(r, *seconds*11025)
	if len(h[0]) < minSampleLength {
		fmt.Fprintln(os.Stderr, "Sample too short")
		os.Exit(2)
	}

	start := len(h[0])
	end := 0
	for k := range h {
		for i, v := range h[k] {
			if v != 0 {
				if i-1 < start {
					start = i
				}
				break
			}
		}
		for i := len(h[k]) - 1; i >= 0; i-- {
			if h[k][i] != 0 {
				if i+1 > end {
					end = i + 1
				}
				break
			}
		}
	}
	for k := range h {
		h[k] = h[k][start:end]
	}

	for attempt := uint(0); attempt < 32; attempt++ {
		if *verbose {
			log.Println("Attempt", attempt)
		}
		candidates, err := fetchCandidates(db, h)
		if err != nil {
			panic(err)
		}
		if *verbose {
			log.Println("Found", len(candidates), "candidates")
		}
		type result struct {
			song  string
			rng   [2]int
			score [len(h)]float64
		}
		var bers []result
		for _, c := range candidates {
			hh, err := fetchHashes(db, c.song, c.rng[0], c.rng[1])
			if err != nil {
				panic(err)
			}
			if len(hh[0]) != len(h[0]) {
				continue
			}

			var res [len(h)]float64
			for k := range hh {
				res[k] = ber(h[k], hh[k])
			}
			bers = append(bers, result{c.song, c.rng, res})
		}
		sort.Slice(bers, func(i, j int) bool {
			var s1, s2 float64
			for k := range bers[i].score {
				s1 += bers[i].score[k]
				s2 += bers[j].score[k]
			}
			return s1 < s2
		})
		if len(bers) > 0 {
			best := (bers[0].score[0] + bers[0].score[1] + bers[0].score[2] + bers[0].score[3]) / float64(len(bers[0].score))
			if best <= threshold {
				for _, r := range bers {
					fmt.Printf("%s [%6d - %6d]: %.2f\n", r.song, r.rng[0], r.rng[1], r.score)
				}
				return
			}
		}

		// found no match, flip one bit and try again
		if attempt > 0 {
			for k := range h {
				for i := range h[k] {
					h[k][i] ^= 1 << (attempt - 1)
				}
			}
		}
		for k := range h {
			for i := range h[k] {
				h[k][i] ^= 1 << attempt
			}
		}
	}
}

const threshold = 0.35

func fetchHashes(db *sql.DB, song string, start, end int) ([4][]uint32, error) {
	var hashes [4][]int64
	row := db.QueryRow(`SELECT array_agg(hash0), array_agg(hash1), array_agg(hash2), array_agg(hash3) FROM hashes WHERE song = $1 AND off >= $2 AND off <= $3`,
		song, start, end)
	if err := row.Scan(pq.Array(&hashes[0]), pq.Array(&hashes[1]), pq.Array(&hashes[2]), pq.Array(&hashes[3])); err != nil {
		return [4][]uint32{}, nil
	}

	var hh [4][]uint32
	for i := range hh {
		hh[i] = make([]uint32, len(hashes[0]))
	}
	for i := range hashes[0] {
		for j := range hh {
			hh[j][i] = uint32(hashes[j][i])
		}
	}
	return hh, nil
}

type candidate struct {
	song string
	rng  [2]int
}

func fetchCandidates(db *sql.DB, h [4][]uint32) ([]candidate, error) {
	candidateScores := map[candidate]int{}
	args := [4][]int32{}
	for i := range h[0] {
		for j := range h {
			args[j] = append(args[j], int32(h[j][i]))
		}
	}

	rows, err := db.Query(`
SELECT song, off, hash0, hash1, hash2, hash3
FROM hashes
WHERE (hash0 = ANY ($1) OR hash1 = ANY ($2) OR hash2 = ANY ($3) OR hash3 = ANY ($4))
      AND hash0 <> 0 AND hash1 <> 0 AND hash2 <> 0 AND hash3<> 0`,
		pq.Array(args[0]), pq.Array(args[1]), pq.Array(args[2]), pq.Array(args[3]))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var song string
		var off int
		var hashes [4]int32
		if err := rows.Scan(&song, &off, &hashes[0], &hashes[1], &hashes[2], &hashes[3]); err != nil {
			return nil, err
		}

		// figure out the offsets in the query hash block, so that we can align it.
		for k := range hashes {
			for i, hh := range args[k] {
				if hh == hashes[k] {
					start, end := off-i, off+len(h[0])-i-1
					candidateScores[candidate{song, [2]int{start, end}}]++
				}
			}
		}
	}

	var candidates []candidate
	for k := range candidateScores {
		candidates = append(candidates, k)
	}

	return candidates, nil
}
