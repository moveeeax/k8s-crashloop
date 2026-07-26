package main

import (
	"fmt"
	"os"

	"github.com/moveeeax/k8s-crashloop/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "k8s-crashloop:", err)
		os.Exit(1)
	}
}
