package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"math/bits"
	"os"
	"sort"
	"strings"

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

	type candidate struct {
		song string
		rng  [2]int
	}
	candidateScores := map[candidate]int{}
	for i := range h[0] {
		var conds []string
		var args []interface{}
		for j := range h {
			if h[j][i] != 0 {
				conds = append(conds, fmt.Sprintf("hash%d = $%d", j, len(args)+1))
				args = append(args, int32(h[j][i]))
			}
		}
		if len(conds) == 0 {
			continue
		}

		rows, err := db.Query(`SELECT song, off FROM hashes WHERE `+strings.Join(conds, " OR "), args...)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var song string
			var off int
			if err := rows.Scan(&song, &off); err != nil {
				panic(err)
			}
			start, end := off-i, off+len(h[0])-i-1
			candidateScores[candidate{song, [2]int{start, end}}]++
		}
	}

	var candidates []candidate
	for k := range candidateScores {
		candidates = append(candidates, k)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidateScores[candidates[i]] > candidateScores[candidates[j]]
	})

	type result struct {
		song  string
		rng   [2]int
		score [len(h)]float64
	}
	var bers []result
	for _, c := range candidates {
		var hashes [len(h)][]int64
		row := db.QueryRow(`SELECT array_agg(hash0), array_agg(hash1), array_agg(hash2), array_agg(hash3) FROM hashes WHERE song = $1 AND off >= $2 AND off <= $3`,
			c.song, c.rng[0], c.rng[1])
		if err := row.Scan(pq.Array(&hashes[0]), pq.Array(&hashes[1]), pq.Array(&hashes[2]), pq.Array(&hashes[3])); err != nil {
			panic(err)
		}

		if len(hashes[0]) != len(h[0]) {
			continue
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
	for _, r := range bers {
		fmt.Printf("%s [%6d - %6d]: %.2f\n", r.song, r.rng[0], r.rng[1], r.score)
	}
}
