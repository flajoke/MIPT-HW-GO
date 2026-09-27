package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	username := os.Getenv("USERNAME")

	if username == "" {
		username = "not found"
	}

	fmt.Println("Username:", username)

	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Println("CLI arguments not provided")
	} else {
		fmt.Println("CLI arguments:")
		for i, arg := range args {
			fmt.Printf(" %d: %s\n", i+1, arg)
		}
	}
	fmt.Println("GO version:", runtime.Version())
}
