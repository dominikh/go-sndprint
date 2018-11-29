package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/bits"
	"os"
	"sort"

	"honnef.co/go/sndprint"

	"github.com/jackc/pgx"
)

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

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

type result struct {
	song  string
	rng   [2]int
	score [4]float64
}

func match(db *pgx.Conn, verbose bool, h [4][]uint32, q [4][]uint32) []result {
	candidates, err := fetchCandidates(db, q)
	if err != nil {
		panic(err)
	}

	checked := map[candidate]bool{}
	if verbose {
		log.Println("Found", len(candidates), "candidates")
	}
	hashes := map[string][4][]uint32{}
	var bers []result
	for _, c := range candidates {
		if checked[c] {
			continue
		}
		checked[c] = true
		hh, ok := hashes[c.song]
		if !ok {
			hh, err = fetchHashes(c.song)
			if err != nil {
				panic(err)
			}
			hashes[c.song] = hh
		}

		for off := 0; off <= c.span; off++ {
			if len(hh[0][off:]) < len(h[0]) {
				continue
			}

			if c.rng[1] > len(hh[0]) {
				continue
			}
			var d [4][]uint32
			for k := range hh {
				d[k] = hh[k][c.rng[0]:c.rng[1]]
			}

			var res [4]float64
			for k := range d {
				res[k] = ber(h[k], d[k][off:len(h[0])])
			}
			rng := [2]int{
				c.rng[0] + off,
				c.rng[0] + off + len(h[0]),
			}
			for _, v := range res {
				if v <= threshold {
					bers = append(bers, result{c.song, rng, res})
					break
				}
			}
		}
	}
	sort.Slice(bers, func(i, j int) bool {
		var s1, s2 float64
		for k := range bers[i].score {
			s1 += bers[i].score[k]
			s2 += bers[j].score[k]
		}
		return s1 < s2
	})
	return bers
}

func printResults(bers []result) {
	prevSong := ""
	for _, r := range bers {
		if r.song == prevSong {
			fmt.Printf("%37s[%6d - %6d]: %.2f\n", "", r.rng[0], r.rng[1], r.score)
		} else {
			prevSong = r.song
			fmt.Printf("%s [%6d - %6d]: %.2f\n", r.song, r.rng[0], r.rng[1], r.score)
		}
	}
}

func main() {
	verbose := flag.Bool("v", false, "Enable verbose output")
	seconds := flag.Int("t", 0, "Max seconds to process")
	flag.Parse()

	const minSampleLength = 256

	if len(flag.Args()) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-query [file]")
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

	bers := match(db, *verbose, h, h)
	if len(bers) > 0 {
		printResults(bers)
		return
	}

	var q [4][]uint32
	for attempt := uint(0); attempt < 32; attempt++ {
		for k := range h {
			for _, v := range h[k] {
				v ^= 1 << attempt
				q[k] = append(q[k], v)
			}
		}
	}

	printResults(match(db, *verbose, h, q))
}

const threshold = 0.35

func fetchHashes(song string) ([4][]uint32, error) {
	path := "/home/dominikh/prj/src/honnef.co/go/sndprint/_fingerprints/" + song
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return [4][]uint32{}, err
	}
	var out [4][]uint32
	for i := 0; i < len(b); i += 4 * 4 {
		d := b[i : i+4*4]
		out[0] = append(out[0], binary.LittleEndian.Uint32(d[0:4]))
		out[1] = append(out[1], binary.LittleEndian.Uint32(d[4:8]))
		out[2] = append(out[2], binary.LittleEndian.Uint32(d[8:12]))
		out[3] = append(out[3], binary.LittleEndian.Uint32(d[12:16]))
	}

	assert(len(out[0]) == len(out[1]))
	assert(len(out[2]) == len(out[3]))
	assert(len(out[0]) == len(out[2]))
	return out, nil
}

type candidate struct {
	song string
	rng  [2]int

	span int
}

func fetchCandidates(db *pgx.Conn, h [4][]uint32) ([]candidate, error) {
	candidateScores := map[candidate]int{}
	args := [4][]int32{}
	for k := range h {
		for _, v := range h[k] {
			args[k] = append(args[k], int32(v))
		}
	}

	hash2off := map[int32][][]int{}
	for k := range h {
		for i, v := range h[k] {
			if hash2off[int32(v)] == nil {
				hash2off[int32(v)] = make([][]int, 4)
			}
			hash2off[int32(v)][k] = append(hash2off[int32(v)][k], i)
		}
	}

	rows, err := db.Query(`
SELECT song, off, hash0, hash1, hash2, hash3
FROM hashes
WHERE (hash0 = ANY ($1) OR hash1 = ANY ($2) OR hash2 = ANY ($3) OR hash3 = ANY ($4))
`,
		args[0], args[1], args[2], args[3])
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
		for k, v := range hashes {
			offs := hash2off[v]
			if offs == nil {
				continue
			}
			for _, i := range offs[k] {
				start, end := off-i, off+len(h[0])-i-1
				if start < 0 {
					continue
				}
				candidateScores[candidate{song: song, rng: [2]int{start, end}}]++
			}
		}
	}

	if len(candidateScores) == 0 {
		return nil, nil
	}

	var candidates []candidate
	for k := range candidateScores {
		candidates = append(candidates, k)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].song != candidates[j].song {
			return candidates[i].song < candidates[j].song
		}
		if candidates[i].rng[0] != candidates[j].rng[0] {
			return candidates[i].rng[0] < candidates[j].rng[0]
		}
		return candidates[i].rng[1] < candidates[j].rng[1]
	})

	merged := make([]candidate, 0, len(candidates))
	merged = append(merged, candidates[0])
	for _, x := range candidates[1:] {
		m := &merged[len(merged)-1]

		if x.song == m.song &&
			x.rng[0] == m.rng[0]+1+m.span &&
			x.rng[1] == m.rng[1]+1 {

			m.rng[1]++
			m.span++
		} else {
			merged = append(merged, x)
		}
	}

	return merged, nil
}
