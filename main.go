package main

import (
	"os"

	"github.com/jewell-lgtm/monkeypuzzle/cmd/mp"
)

func main() {
	if err := mp.Execute(); err != nil {
		os.Exit(1)
	}
}
