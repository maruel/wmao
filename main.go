package main

import (
	"context"
	"fmt"
	"os"
)

func mainImpl() error {
	fmt.Println("WIP")
	return nil
}

func main() {
	if err := mainImpl(); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "wmao: %v\n", err)
		os.Exit(1)
	}
}
