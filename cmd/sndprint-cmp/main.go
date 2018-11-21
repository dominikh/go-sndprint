// Command sndprint-cmp compares two audio files and reports whether they are perceptually identical.
package main

import (
	"flag"
	"fmt"
	"math/bits"
	"os"

	"honnef.co/go/sndprint"
)

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func main() {
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if len(flag.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: sndprint-cmp [-v] <file1> <file2>")
		os.Exit(2)
	}

	f1, err := os.Open(flag.Args()[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not open file:", err)
		os.Exit(2)
	}
	f2, err := os.Open(flag.Args()[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not open file:", err)
		os.Exit(2)
	}

	fmt.Println("hashing 1")
	h1 := sndprint.Hash(f1)
	fmt.Println("hashing 2")
	h2 := sndprint.Hash(f2)
	fmt.Println("comparing")
	n := min(len(h1), len(h2))
	total := max(len(h1), len(h2)) * 32

	e := 0
	for i := 0; i < n; i++ {
		e += bits.OnesCount32(h1[i] ^ h2[i])
	}
	e += abs(len(h1)-len(h2)) * 32
	ber := float64(e) / float64(total)
	if ber > 0.25 {
		if *verbose {
			fmt.Printf("BER = %.2f - not identical\n", ber)
		}
		os.Exit(1)
	}
	if *verbose {
		fmt.Printf("BER = %.2f - identical\n", ber)
	}
	os.Exit(0)
}
