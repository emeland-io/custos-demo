package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"notebook/test/e2e/internal"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	if err := internal.CheckPrerequisites(exec.LookPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "up":
		if err := internal.Up(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "up failed:", err)
			os.Exit(1)
		}
	case "down":
		deleteCluster := flag.NewFlagSet("down", flag.ExitOnError)
		deleteClusterFlag := deleteCluster.Bool("delete-cluster", false, "also delete the kind cluster")
		_ = deleteCluster.Parse(os.Args[2:])
		if err := internal.Down(ctx, *deleteClusterFlag); err != nil {
			fmt.Fprintln(os.Stderr, "down failed:", err)
			os.Exit(1)
		}
	case "baseurl":
		state, err := internal.LoadState()
		if err != nil {
			fmt.Fprintln(os.Stderr, "no environment is up:", err)
			os.Exit(1)
		}
		fmt.Println(state.BaseURL)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: devcluster <up|down [--delete-cluster]|baseurl>")
}
