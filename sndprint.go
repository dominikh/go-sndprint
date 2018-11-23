package sndprint

// #cgo pkg-config: fftw3
// #include <fftw3.h>
import "C"

import (
	"bufio"
	"io"
	"log"
	"math"
	"unsafe"
)

/*
CREATE TABLE hashes (
  id SERIAL PRIMARY KEY,
  hash0 INTEGER NOT NULL,
  hash1 INTEGER NOT NULL,
  hash2 INTEGER NOT NULL,
  hash3 INTEGER NOT NULL,
  song UUID NOT NULL,
  off INTEGER NOT NULL
);

CREATE INDEX ON hashes (hash0);
CREATE INDEX ON hashes (hash1);
CREATE INDEX ON hashes (hash2);
CREATE INDEX ON hashes (hash3);
CREATE UNIQUE INDEX ON hashes (song, off);
*/

// These bins correspond to 33 bins in the range 20–5500 Hz with
// logarithmic spacing between them, akin to a Bark scale. These bins
// have been coarsely aligned to the FFT bins.
var fftBins = [33][2]int{
	{8, 8},
	{9, 10},
	{11, 12},
	{13, 14},
	{15, 17},
	{18, 20},
	{21, 24},
	{25, 29},
	{30, 34},
	{35, 40},
	{41, 48},
	{49, 57},
	{58, 67},
	{68, 80},
	{81, 95},
	{96, 113},
	{114, 134},
	{135, 159},
	{160, 188},
	{189, 223},
	{224, 265},
	{266, 313},
	{314, 372},
	{373, 441},
	{442, 523},
	{524, 620},
	{621, 735},
	{736, 872},
	{873, 103},
	{1035, 1226},
	{1227, 1453},
	{1454, 1723},
	{1724, 2043},
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

type complex struct {
	real, imag float64
}

func (x complex) Abs() float64 { return math.Hypot(x.real, x.imag) }

func Hash(r io.Reader, maxSamples int) [4][]uint32 {
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

	in := C.fftw_alloc_real(windowSize)
	out := C.fftw_alloc_complex(windowSize) // XXX
	plan := C.fftw_plan_dft_r2c_1d(windowSize, in, out, 0)
	tmp := (*[math.MaxInt32]float64)(unsafe.Pointer(in))[:windowSize]
	dft := (*[math.MaxInt32]complex)(unsafe.Pointer(out))[:windowSize]

	var bandEnergies [][len(fftBins)]float64
	totalSamples := 0
	for maxSamples == 0 || totalSamples < maxSamples {
		totalSamples += step
		for i, sample := range samples {
			tmp[i] = hamming[i] * sample
		}

		// dft := fft.FFTReal(tmp)
		C.fftw_execute(plan)
		var energies [len(fftBins)]float64
		for bin, binLimits := range fftBins {
			for fftBin := binLimits[0]; fftBin <= binLimits[1]; fftBin++ {
				energies[bin] += dft[fftBin].Abs()
			}
		}
		bandEnergies = append(bandEnergies, energies)

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

	const L = 16
	const K = 4
	dctIn := C.fftw_alloc_real(L)
	dctOut := C.fftw_alloc_real(L)
	dctPlan := C.fftw_plan_r2r_1d(L, dctIn, dctOut, C.FFTW_REDFT10, 0)
	C := func(frame, band int) [L]float64 {
		reals := (*[L]float64)(unsafe.Pointer(dctIn))
		for i := range reals {
			reals[i] = bandEnergies[frame+i][band]
		}

		C.fftw_execute(dctPlan)

		return *(*[L]float64)(unsafe.Pointer(dctOut))
	}
	ED := func(frame, band, k int) float64 {
		return C(frame, band)[k] - C(frame, band+1)[k] -
			(C(frame-L, band)[k] - C(frame-L, band+1)[k])
	}
	F := func(frame, k int) uint32 {
		var fp uint32
		for band := 0; band < 32; band++ {
			if ED(frame, band, k) > 0 {
				fp |= 1 << uint(band)
			}
		}
		return fp
	}

	var res [K][]uint32
	for i := L; i < len(bandEnergies)-L; i++ {
		for k := range res {
			res[k] = append(res[k], F(i, k))
		}
	}

	return res
}
