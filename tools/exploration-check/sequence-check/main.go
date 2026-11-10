package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: sequence-check")
		os.Exit(2)
	}
	report := runSuite()
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "could not encode sequence check report:", err)
		os.Exit(2)
	}
	if !report.Valid {
		os.Exit(1)
	}
}
