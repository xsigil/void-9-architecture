// Void Architecture™ Sovereign CLI Engine (void)
// Author: parorafia (@xsigil)
// Domain: Resilient Systems, Cyber Defense & Sovereign Application Architecture
// License: MIT

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed all:templates scripts/*
var embeddedAssets embed.FS

func printHelp() {
	fmt.Println(`Void Sovereign Engine (void)
Hyper-Minimalist Sovereign Architecture & Confinement Perimeter

USAGE:
  void scaffold <app dir>
  void ldd <linked executable> <lib dir>
  void jail [--bind=<src>:<dst>[:ro],...] --lib <lib dir> <exec> [args...]
  void export [--bind=<src>:<dst>[:ro],...] --lib <lib dir> --output <tarball|dir> <exec>

SUBCOMMANDS:
  scaffold  Generate a pure DDD Void skeleton directory tree
  ldd       Recursively resolve and collect dynamic ELF libraries and loader
  jail      Execute target binary inside volatile unshare/chroot RAM jail
  export    Package self-contained standalone rootfs archive or directory
`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "scaffold":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "[-] Error: <app dir> argument required.")
			fmt.Fprintln(os.Stderr, "    Usage: void scaffold <app dir>")
			os.Exit(1)
		}
		handleScaffold(os.Args[2])

	case "ldd":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "[-] Error: <linked executable> and <lib dir> required.")
			fmt.Fprintln(os.Stderr, "    Usage: void ldd <linked executable> <lib dir>")
			os.Exit(1)
		}
		handleLdd(os.Args[2], os.Args[3])

	case "jail":
		handleJail(os.Args[2:])

	case "export":
		handleExport(os.Args[2:])

	case "-h", "--help", "help":
		printHelp()

	default:
		fmt.Fprintf(os.Stderr, "[-] Unknown subcommand: %s\n", os.Args[1])
		printHelp()
		os.Exit(1)
	}
}

func handleScaffold(appDir string) {
	start := time.Now()
	appName := filepath.Base(appDir)
	fmt.Printf("[+] Materializing Void Sovereign Architecture for: %s (%s)\n", appName, appDir)

	// SPEC.md に定義された骨格ディレクトリを生成
	dirs := []string{
		filepath.Join(appDir, "bin"),
		filepath.Join(appDir, "lib"),
		filepath.Join(appDir, "rootfs", "dev"),
		filepath.Join(appDir, "rootfs", "proc"),
		filepath.Join(appDir, "rootfs", "etc"),
		filepath.Join(appDir, "rootfs", "tmp"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Failed to create directory %s: %v\n", d, err)
			os.Exit(1)
		}
	}

	// 埋め込みテンプレートファイルの展開
	templatePrefix := "templates"
	entriesFound := 0

	err := fs.WalkDir(embeddedAssets, templatePrefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == templatePrefix {
			return nil
		}

		relPath, err := filepath.Rel(templatePrefix, path)
		if err != nil {
			return err
		}

		// void 自身の内部テンプレートはプロジェクト生成対象から除外
		if relPath == "jail.sh.tmpl" {
			return nil
		}

		destRelPath := strings.TrimSuffix(relPath, ".tmpl")
		destPath := filepath.Join(appDir, destRelPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		data, err := embeddedAssets.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		content := strings.ReplaceAll(string(data), "{{MODULE_NAME}}", appName)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		perm := os.FileMode(0644)
		if strings.HasSuffix(destRelPath, ".sh") {
			perm = 0755
		}

		if err := os.WriteFile(destPath, []byte(content), perm); err != nil {
			return fmt.Errorf("failed to write %s: %w", destPath, err)
		}

		entriesFound++
		fmt.Printf("  -> [SOVEREIGN NODE] %s\n", destRelPath)
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] Scaffolding failed: %v\n", err)
		os.Exit(1)
	}

	// Voidfile の初期配置 (SPEC.md 準拠)
	voidfilePath := filepath.Join(appDir, "Voidfile")
	if _, err := os.Stat(voidfilePath); os.IsNotExist(err) {
		defaultVoidfile := fmt.Sprintf("# Voidfile: %s\nEXEC=./bin/%s\nLIB=./lib\n", appName, appName)
		_ = os.WriteFile(voidfilePath, []byte(defaultVoidfile), 0644)
	}

	duration := time.Since(start)
	fmt.Printf("\n[+] Void architecture materialized successfully in %v.\n", duration)
	fmt.Printf("    cd %s\n", appDir)
	fmt.Printf("    go mod tidy\n")
}

func handleLdd(execPath, libDir string) {
	fmt.Printf("[+] Analyzing ELF dynamic dependencies for: %s\n", execPath)
	fmt.Printf("[+] Target library collection directory: %s\n", libDir)

	if err := os.MkdirAll(libDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Failed to prepare lib directory: %v\n", err)
		os.Exit(1)
	}

	if err := ResolveAndCollectLibraries(execPath, libDir); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Dependency resolution failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[+] Dynamic dependencies successfully materialized.")
}

func handleJail(args []string) {
	if err := RunJail(args); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Void jail execution error: %v\n", err)
		os.Exit(1)
	}
}

func handleExport(args []string) {
	fmt.Printf("[*] void export: packaging rootfs standalone (stub)\n")
	// TODO: Phase 4 で tarball / rootfs 出力を実装
}
