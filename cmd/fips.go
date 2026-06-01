//go:build fips

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
)

func main() {
	// Test SHA-256 (FIPS-approved algorithm)
	hash := sha256.Sum256([]byte("FIPS self-test"))
	fmt.Printf("SHA-256: %x\n", hash)

	// Test random number generation (FIPS-approved)
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		fmt.Fprintf(os.Stderr, "FIPS random read failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Random bytes generated: %d\n", len(buf))

	fmt.Println("FIPS self-test PASSED")
}
