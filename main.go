package main

import (
	"fmt"
	"os"
)

func printUsage() {
	fmt.Println("Void Architecture CLI Specification (void)")
	fmt.Println("Usage:")
	fmt.Println("  void scaffold [--go] <app dir>")
	fmt.Println("  void ldd <linked executable> <lib dir>")
	fmt.Println("  void jail [--bind=<src>:<dst>[:ro],...] --lib <lib dir> <exec> [args...]")
	fmt.Println("  void export [--bind=<src>:<dst>[:ro],...] --lib <lib dir> --output <tarball|dir> <exec>")
	fmt.Println("\nSubcommands:")
	fmt.Println("  scaffold  Generate a pure Void skeleton directory tree (Default: OCaml, --go for Go)")
	fmt.Println("  ldd       Incremental shared library and dynamic linker tracker")
	fmt.Println("  jail      Execute containerless isolated process inside namespaces")
	fmt.Println("  export    Export standalone chroot/unshare rootfs or tarball")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	args := os.Args[2:]

	switch subcommand {
	case "scaffold":
		if err := RunScaffold(args); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error: %v\n", err)
			os.Exit(1)
		}

	case "ldd":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "[-] Usage: void ldd <linked executable> <lib dir>")
			os.Exit(1)
		}
		if err := ResolveAndCollectLibraries(args[0], args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error: %v\n", err)
			os.Exit(1)
		}

	case "jail":
		if err := RunJail(args); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error: %v\n", err)
			os.Exit(1)
		}

	case "export":
		if err := RunExport(args); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error: %v\n", err)
			os.Exit(1)
		}

	case "-h", "--help", "help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "[-] Unknown subcommand: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}
