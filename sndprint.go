package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/bits"
	"math/cmplx"
	"os"

	"github.com/mjibson/go-dsp/fft"
)

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

func hashes(path string) []uint32 {
	// OPT(dh): of course we could stream the data
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}
	samples := make([]float64, len(data)/depth)
	for i := 0; i < len(data)-2; i += 2 {
		samples[i/2] = float64(int16(uint16(data[i]) | uint16(data[i+1])<<8))
	}

	tmp := make([]float64, windowSize)

	var hashes []uint32
	var prevEnergies [len(fftBins)]float64

	for i := 0; i < len(samples)-windowSize; i += step {
		for j := range samples[i : i+windowSize] {
			tmp[j] = hamming[j] * samples[i+j]
		}
		freqc := fft.FFTReal(tmp)

		var energies [len(fftBins)]float64
		for bin, binLimits := range fftBins {
			for fftBin := binLimits[0]; fftBin <= binLimits[1]; fftBin++ {
				energies[bin] += cmplx.Abs(freqc[fftBin])
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
	}

	return hashes
}

func main() {
	h1 := hashes(os.Args[1])
	return
	h2 := hashes(os.Args[2])

	total := len(h1) * 32
	errors := 0

	N := len(h1)
	if len(h2) < N {
		N = len(h2)
	}
	for i := 0; i < N; i++ {
		errors += bits.OnesCount32(h1[i] ^ h2[i])
	}
	fmt.Println(float64(errors) / float64(total))
}
