package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"honnef.co/go/sndprint"
	"honnef.co/go/sndprint/sndprintdb"
)

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

func printResults(bers []sndprintdb.Result) {
	prevSong := ""
	for _, r := range bers {
		if r.Song == prevSong {
			fmt.Printf("%37s[%6d - %6d]: %.2f\n", "", r.Range[0], r.Range[1], r.Score)
		} else {
			prevSong = r.Song
			fmt.Printf("%s [%6d - %6d]: %.2f\n", r.Song, r.Range[0], r.Range[1], r.Score)
		}
	}
}

func main() {
	seconds := flag.Int("t", 0, "Max seconds to process")
	flag.Parse()

	const minSampleLength = 256

	if len(flag.Args()) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-query [file]")
		os.Exit(2)
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

	db, err := sndprintdb.Open("/home/dominikh/prj/src/honnef.co/go/sndprint/_fingerprints/")
	if err != nil {
		panic(err) // XXX
	}
	res, err := db.Match(h)
	if err != nil {
		panic(err) // XXX
	}
	printResults(res)
}
