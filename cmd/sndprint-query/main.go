package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"honnef.co/go/sndprint"
	"honnef.co/go/sndprint/cmdutil"
	"honnef.co/go/sndprint/sndprintdb"
)

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

const minSampleLength = 256

func main() {
	seconds := flag.Int("t", 0, "Max seconds to process")
	flag.Parse()

	if len(flag.Args()) > 2 {
		cmdutil.Usage("Usage: sndprint-query [file]")
	}

	db, err := cmdutil.DB()
	if err != nil {
		cmdutil.Die("Could not open fingerprint database:", err)
	}

	var r io.Reader = os.Stdin
	if len(flag.Args()) == 2 {
		f, err := os.Open(flag.Args()[1])
		if err != nil {
			cmdutil.Die("Could not open file:", err)
		}
		defer f.Close()
		r = f
	}
	if *seconds > 0 {
		r = &io.LimitedReader{R: r, N: int64(*seconds * 11025 * 2)}
	}

	h := sndprint.Hash(r)
	if len(h[0]) < minSampleLength {
		cmdutil.Die("Sample too short")
	}

	res, err := db.Match(h)
	if err != nil {
		cmdutil.Die("Couldn't search database:", err)
	}
	printResults(res)
}
