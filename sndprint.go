package sndprint

import (
	"bufio"
	"io"
	"log"
	"math"
	"math/bits"
	"math/cmplx"

	"github.com/mjibson/go-dsp/fft"
)

/*
CREATE TABLE hashes (
  id SERIAL PRIMARY KEY,
  hash INTEGER NOT NULL,
  song UUID NOT NULL,
  off INTEGER NOT NULL
);

CREATE INDEX ON hashes (hash);
CREATE INDEX ON hashes (song, off);
*/

// These bins correspond to 33 bins in the range 300–3000 Hz with
// logarithmic spacing between them, akin to a Bark scale. These bins
// have been coarsely aligned to the FFT bins.
var fftBins = [33][2]int{
	{112, 119},
	{120, 128},
	{129, 137},
	{138, 147},
	{148, 158},
	{159, 169},
	{170, 181},
	{182, 195},
	{196, 209},
	{210, 224},
	{225, 240},
	{241, 257},
	{258, 276},
	{277, 296},
	{297, 317},
	{318, 340},
	{341, 365},
	{366, 391},
	{392, 419},
	{420, 450},
	{451, 482},
	{483, 517},
	{518, 555},
	{556, 595},
	{596, 637},
	{638, 683},
	{684, 733},
	{734, 786},
	{787, 843},
	{844, 904},
	{905, 969},
	{970, 1039},
	{1040, 1115},
}

const (
	windowSize = 4096
	step       = 128
	rate       = 11025
	depth      = 2
)

var hamming = make([]float64, windowSize)

func init() {
	const M = windowSize
	for n := 0; n < windowSize; n++ {
		hamming[n] = 0.54 - 0.46*math.Cos((2*math.Pi*float64(n))/(M-1))
	}
}

func Hash(r io.Reader) []uint32 {
	br := bufio.NewReader(r)

	// Read initial set of samples
	b := make([]byte, windowSize*depth)
	if _, err := io.ReadFull(br, b); err != nil {
		log.Fatal(err)
	}
	samples := make([]float64, windowSize)
	for i := 0; i < len(b)-2; i += 2 {
		samples[i/2] = float64(int16(uint16(b[i]) | uint16(b[i+1])<<8))
	}

	b = b[:step*depth]
	var hashes []uint32
	var prevEnergies [len(fftBins)]float64
	tmp := make([]float64, windowSize)
	for {
		for i, sample := range samples {
			tmp[i] = hamming[i] * sample
		}

		dft := fft.FFTReal(tmp)
		var energies [len(fftBins)]float64
		for bin, binLimits := range fftBins {
			for fftBin := binLimits[0]; fftBin <= binLimits[1]; fftBin++ {
				energies[bin] += cmplx.Abs(dft[fftBin])
			}
		}

		var hash uint32
		for bit := uint(0); bit < 32; bit++ {
			if energies[bit]-energies[bit+1]-(prevEnergies[bit]-prevEnergies[bit+1]) > 0 {
				hash |= 1 << bit
			}
		}
		hashes = append(hashes, hash)
		prevEnergies = energies

		// Slide window forward
		if n, err := io.ReadFull(br, b); err != nil {
			if err == io.EOF {
				if n == 0 {
					break
				}
			} else if err == io.ErrUnexpectedEOF {
				if n == 0 {
					break
				}
				for i := n; i < len(b); i++ {
					b[i] = 0
				}
			} else {
				panic(err)
			}
		}
		copy(samples, samples[step:])
		for i := 0; i < len(b)-2; i += 2 {
			samples[len(samples)-step+i/2] = float64(int16(uint16(b[i]) | uint16(b[i+1])<<8))
		}
	}

	return hashes
}

func ber(s1, s2 []uint32) float64 {
	e := 0
	for i := range s1 {
		e += bits.OnesCount32(s1[i] ^ s2[i])
	}
	return float64(e) / (256 * 32)
}

const berCutoff = 0.25

// func main() {
// 	db, err := sql.Open("postgres", "")
// 	if err != nil {
// 		panic(err)
// 	}

// 	type checkKey struct {
// 		song     string
// 		offStart int64
// 		offEnd   int64
// 	}
// 	checked := map[checkKey]float64{}
// 	h := hashes("/tmp/out3.raw")
// 	h = h[:256]
// 	for i, hash := range h {
// 		rows, err := db.Query(`SELECT song, array_agg(off) FROM hashes WHERE hash = $1 GROUP BY song`, int32(hash))
// 		if err != nil {
// 			panic(err)
// 		}
// 		for rows.Next() {
// 			var song []byte
// 			var offs []int64
// 			if err := rows.Scan(&song, pq.Array(&offs)); err != nil {
// 				panic(err)
// 			}

// 			// OPT(dh): we can thin out the result set by coalescing sequential matches for the same song
// 			for _, off := range offs {
// 				if int64(i) > off {
// 					// Impossible match, sample_n can't ever occur before song_n
// 					continue
// 				}

// 				{
// 					start, end := off-int64(i), off+256-int64(i)
// 					if _, ok := checked[checkKey{string(song), start, end}]; ok {
// 						// We've already checked this segment
// 						continue
// 					}
// 					rows, err := db.Query(`SELECT hash FROM hashes WHERE song = $1 AND off >= $2 AND off <= $3 LIMIT 256`, song, off-int64(i), off+256-int64(i))
// 					if err != nil {
// 						panic(err)
// 					}
// 					hashes := make([]uint32, 0, 256)
// 					for rows.Next() {
// 						var hash int32
// 						if err := rows.Scan(&hash); err != nil {
// 							panic(err)
// 						}
// 						hashes = append(hashes, uint32(hash))
// 					}
// 					if rows.Err() != nil {
// 						panic(rows.Err())
// 					}
// 					if len(hashes) != 256 {
// 						// Too short
// 						continue
// 					}

// 					checked[checkKey{string(song), start, end}] = ber(h, hashes)
// 				}
// 			}
// 		}
// 		if rows.Err() != nil {
// 			panic(rows.Err())
// 		}
// 	}
// 	spew.Dump(checked)
// }
