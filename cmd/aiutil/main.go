package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"qazna.org/internal/ai"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "verify":
		runVerify()
	default:
		usage()
	}
}

func runVerify() {
	if len(os.Args) < 3 {
		usage()
	}
	target := os.Args[2]
	var verifier ai.Verifier
	switch target {
	case "basic":
		verifier = ai.BasicVerifier()
	default:
		fmt.Fprintf(os.Stderr, "unknown verify target %q\n", target)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := verifier.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "verify %s failed: %v\n", target, err)
		os.Exit(1)
	}
	fmt.Printf("verify %s: PASS\n", target)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s verify <target>\n", os.Args[0])
	os.Exit(1)
}
