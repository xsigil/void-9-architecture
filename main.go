package main

import (
	"fmt"
	"os"
)

func printUsage() {
	fmt.Println("Void-9 Architecture Tool")
	fmt.Println("Usage:")
	fmt.Println("  void ldd <linked executable> <lib dir>")
	fmt.Println("  void jail [-size <size>] -lib <dir> [-bind <mappings>] [--debug] [--export] <binary...> [-- <args...>]")
	fmt.Println("  void pkg <package.pkg.tar.zst> <output-rootfs-dir>")
	fmt.Println("  void export [--bind=<src>:<dst>[:ro],...] --lib <lib dir> --output <tarball|dir> <exec>")
	fmt.Println("\nCommands:")
	fmt.Println("  ldd       Incremental shared library and dynamic linker tracker")
	fmt.Println("  jail      Volatile RAM-based unshared containment runner")
	fmt.Println("  pkg       Unpack Arch package into rootfs and collect ELF dependencies")
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
	case "pkg":
		if err := RunPkg(args); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if err := RunExport(args); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error: %v\n", err)
			os.Exit(1)
		}
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "[-] Unknown subcommand: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}
