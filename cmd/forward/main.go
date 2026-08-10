package main

import (
	"fmt"
	"os"

	"github.com/forward/forward/internal/agentcli"
)

func main() {
	if err := agentcli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
