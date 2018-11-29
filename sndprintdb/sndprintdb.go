package sndprintdb

import (
	"encoding/binary"
	"encoding/hex"
	"io/ioutil"
	"math/bits"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx"
)

type Result struct {
	Song  string
	Range [2]int
	Score [4]float64
}

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

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

type DB struct {
	conn *pgx.Conn
	dir  string
}

func Open(path string) (*DB, error) {
	conf, err := pgx.ParseEnvLibpq()
	if err != nil {
		return nil, err
	}
	db, err := pgx.Connect(conf)
	if err != nil {
		return nil, err
	}
	return &DB{
		conn: db,
		dir:  path,
	}, nil
}

func trim(h [4][]uint32) {
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
}

func (db *DB) Match(hashes [4][]uint32) ([]Result, error) {
	trim(hashes)

	bers, err := db.match(hashes, hashes)
	if err != nil {
		return nil, err
	}
	if len(bers) > 0 {
		return bers, nil
	}

	var q [4][]uint32
	for attempt := uint(0); attempt < 32; attempt++ {
		for k := range hashes {
			for _, v := range hashes[k] {
				v ^= 1 << attempt
				q[k] = append(q[k], v)
			}
		}
	}
	return db.match(hashes, q)
}

const threshold = 0.35

type candidate struct {
	song string
	rng  [2]int

	span int
}

func (db *DB) candidates(h [4][]uint32) ([]candidate, error) {
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

	rows, err := db.conn.Query(`
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

func (db *DB) match(h [4][]uint32, q [4][]uint32) ([]Result, error) {
	candidates, err := db.candidates(q)
	if err != nil {
		return nil, err
	}

	checked := map[candidate]bool{}
	hashes := map[string][4][]uint32{}
	var bers []Result
	for _, c := range candidates {
		if checked[c] {
			continue
		}
		checked[c] = true
		hh, ok := hashes[c.song]
		if !ok {
			hh, err = db.Hashes(c.song)
			if err != nil {
				return nil, err
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
					bers = append(bers, Result{c.song, rng, res})
					break
				}
			}
		}
	}
	sort.Slice(bers, func(i, j int) bool {
		var s1, s2 float64
		for k := range bers[i].Score {
			s1 += bers[i].Score[k]
			s2 += bers[j].Score[k]
		}
		return s1 < s2
	})
	return bers, nil
}

func (db *DB) Hashes(song string) ([4][]uint32, error) {
	path := filepath.Join(db.dir, song)
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

func (db *DB) AddSong(song string, hashes [4][]uint32) (err error) {
	f, err := os.Create(filepath.Join(db.dir, song))
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			return
		}
		err1 := f.Sync()
		err2 := f.Close()
		if err1 != nil {
			err = err1
		} else {
			err = err2
		}
	}()
	var data [][]interface{}
	for i := range hashes[0] {
		b := make([]byte, 4*4)
		binary.LittleEndian.PutUint32(b[0:4], uint32(hashes[0][i]))
		binary.LittleEndian.PutUint32(b[4:8], uint32(hashes[1][i]))
		binary.LittleEndian.PutUint32(b[8:12], uint32(hashes[2][i]))
		binary.LittleEndian.PutUint32(b[12:16], uint32(hashes[3][i]))
		_, err = f.Write(b)
		if err != nil {
			return err
		}

		data = append(data, []interface{}{
			int32(hashes[0][i]),
			int32(hashes[1][i]),
			int32(hashes[2][i]),
			int32(hashes[3][i]),
			i,
			parseMBID(song),
		})
	}

	_, err = db.conn.CopyFrom(pgx.Identifier{"hashes"}, []string{"hash0", "hash1", "hash2", "hash3", "off", "song"}, pgx.CopyFromRows(data))
	return err
}
