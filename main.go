// Void Architecture™ Sovereign CLI Engine (void)
// Author: parorafia (@xsigil)
// Domain: Resilient Systems, Cyber Defense & Sovereign Application Architecture
// License: MIT

package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed all:templates scripts/*
var embeddedAssets embed.FS

// Default jail script embedded as fallback and canonical export source
const defaultJailScript = `#!/usr/bin/env bash
# Void Architecture™ Kernel Confinement Perimeter
# Volatile RAM Jail (tmpfs + unshare + chroot + /data conduit)
set -euo pipefail

JAIL_DIR="${JAIL_DIR:-/run/void-jail}"
APP_BIN="${APP_BIN:-./bin/app}"
HOST_DATA_DIR="${HOST_DATA_DIR:-$(pwd)/data}"
TMPFS_SIZE="${TMPFS_SIZE:-64M}"

if [[ $EUID -ne 0 ]]; then
    echo "[-] Void containment requires root (sudo) privileges." >&2
    exit 1
fi

if [[ ! -f "${APP_BIN}" ]]; then
    echo "[-] Target executable ${APP_BIN} not found." >&2
    exit 1
fi

echo "[*] Materializing volatile memory jail at ${JAIL_DIR} (${TMPFS_SIZE} tmpfs)..."
mkdir -p "${JAIL_DIR}"
mount -t tmpfs -o "size=${TMPFS_SIZE},nodev,nosuid" tmpfs "${JAIL_DIR}"

mkdir -p "${JAIL_DIR}"/{bin,data,lib,lib64,proc,dev,etc,tmp}
cp "${APP_BIN}" "${JAIL_DIR}/bin/app"
chmod 755 "${JAIL_DIR}/bin/app"

# Minimal device nodes
mknod -m 666 "${JAIL_DIR}/dev/null" c 1 3 2>/dev/null || true
mknod -m 666 "${JAIL_DIR}/dev/zero" c 1 5 2>/dev/null || true
mknod -m 666 "${JAIL_DIR}/dev/urandom" c 1 9 2>/dev/null || true

# Minimal identity & resolver configurations
[[ -f /etc/resolv.conf ]] && cp -a /etc/resolv.conf "${JAIL_DIR}/etc/" 2>/dev/null || true

# The Single Conduit: Bind mount host SQLite data directory (/data only)
echo "[*] Projecting single state conduit: ${HOST_DATA_DIR} -> /data..."
mkdir -p "${HOST_DATA_DIR}"
mount --bind "${HOST_DATA_DIR}" "${JAIL_DIR}/data"

cleanup() {
    echo -e "\n[*] Collapsing Void jail back to non-existence..."
    umount -l "${JAIL_DIR}/data" 2>/dev/null || true
    umount -l "${JAIL_DIR}" 2>/dev/null || true
    rm -rf "${JAIL_DIR}"
    echo "[*] Containment dissolved. 0 bytes persistent on host."
}
trap cleanup EXIT INT TERM

echo "[*] Executing binary in unshared kernel namespaces (NEWNS, NEWPID, NEWIPC, NEWUTS)..."
unshare --mount --pid --ipc --uts --fork chroot "${JAIL_DIR}" /bin/app
`

func printHelp() {
	fmt.Println(`Void Sovereign Engine (void)
Hyper-Minimalist Sovereign Architecture & Confinement Perimeter

USAGE:
  void <app-name>                     Scaffold a pure DDD Void web application
  void export                         Export transparent unshare/chroot jail bash script
  void jail <binary> [--bind=<path>]  Execute binary inside volatile unshare/chroot RAM jail

EXAMPLES:
  void sovereign-core
  void export > run_jail.sh
  sudo void jail ./bin/app --bind=/srv/app/data`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "export":
		handleExport()
	case "jail":
		handleJail(os.Args[2:])
	case "-h", "--help", "help":
		printHelp()
	default:
		// Any positional string argument is treated as the project target name
		if strings.HasPrefix(cmd, "-") {
			fmt.Printf("[-] Unknown option: %s\n", cmd)
			printHelp()
			os.Exit(1)
		}
		scaffoldProject(cmd)
	}
}

func handleExport() {
	// Try reading embedded script if present, otherwise fallback to default string
	data, err := embeddedAssets.ReadFile("scripts/run_jail.sh")
	if err == nil && len(data) > 0 {
		os.Stdout.Write(data)
		return
	}
	fmt.Print(defaultJailScript)
}

func handleJail(args []string) {
	fsFlags := flag.NewFlagSet("jail", flag.ExitOnError)
	bindPath := fsFlags.String("bind", "./data", "Host directory for persistent SQLite /data conduit")
	tmpfsSize := fsFlags.String("size", "64M", "Size of volatile tmpfs RAM filesystem")
	fsFlags.Parse(args)

	targets := fsFlags.Args()
	if len(targets) < 1 {
		fmt.Println("[-] Error: target executable binary required.")
		fmt.Println("    Usage: void jail <binary> [--bind=./data] [--size=64M]")
		os.Exit(1)
	}

	targetBin, err := filepath.Abs(targets[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] Failed to resolve binary path: %v\n", err)
		os.Exit(1)
	}

	absBind, err := filepath.Abs(*bindPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] Failed to resolve bind path: %v\n", err)
		os.Exit(1)
	}

	scriptContent := defaultJailScript
	if data, err := embeddedAssets.ReadFile("scripts/run_jail.sh"); err == nil && len(data) > 0 {
		scriptContent = string(data)
	}

	cmd := exec.Command("bash", "-c", scriptContent)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("APP_BIN=%s", targetBin),
		fmt.Sprintf("HOST_DATA_DIR=%s", absBind),
		fmt.Sprintf("TMPFS_SIZE=%s", *tmpfsSize),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Void jail execution finished: %v\n", err)
		os.Exit(1)
	}
}

func scaffoldProject(appName string) {
	start := time.Now()
	fmt.Printf("[+] Materializing Void Sovereign Architecture for: %s\n", appName)

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

		// Strip .tmpl suffix if present for destination filenames
		destRelPath := strings.TrimSuffix(relPath, ".tmpl")
		destPath := filepath.Join(appName, destRelPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		data, err := embeddedAssets.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		// Replace {{MODULE_NAME}} placeholder
		content := strings.ReplaceAll(string(data), "{{MODULE_NAME}}", appName)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Determine executable permissions
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

	if entriesFound == 0 {
		fmt.Fprintf(os.Stderr, "[-] Warning: No templates found in embedded filesystem under %s/\n", templatePrefix)
	}

	duration := time.Since(start)
	fmt.Printf("\n[+] Void architecture materialized successfully in %v.\n", duration)
	fmt.Printf("    cd %s\n", appName)
	fmt.Printf("    go mod tidy\n")
	fmt.Printf("    go run .\n")
}
