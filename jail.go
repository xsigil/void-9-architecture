package main

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed templates/jail.sh.tmpl
var jailScriptTemplate string

// BindMount represents a host-to-jail projected filesystem path
type BindMount struct {
	Src      string
	Dst      string
	ReadOnly bool
}

// JailTemplateData defines the parameters injected into templates/jail.sh.tmpl
type JailTemplateData struct {
	JailID     string
	Size       string
	LibDir     string
	User       string
	BindMounts []BindMount
	ExecBins   []string // 渡されたすべてのバイナリ絶対パス (ExecBins[0] がエントリポイント)
	TargetBin  string   // エントリポイントのベース名
	ExecArgs   []string // 実行引数
}

func parseBindFlags(raw string) ([]BindMount, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var mounts []BindMount
	entries := strings.Split(raw, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("invalid bind syntax '%s', expected <src>:<dst>[:ro]", entry)
		}

		src, err := filepath.Abs(parts[0])
		if err != nil {
			return nil, fmt.Errorf("failed to resolve absolute path for %s: %w", parts[0], err)
		}

		dst := parts[1]
		if !strings.HasPrefix(dst, "/") {
			dst = "/" + dst
		}

		ro := false
		if len(parts) == 3 {
			if parts[2] == "ro" {
				ro = true
			} else if parts[2] != "rw" {
				return nil, fmt.Errorf("invalid mount flag '%s', expected 'ro' or 'rw'", parts[2])
			}
		}

		mounts = append(mounts, BindMount{
			Src:      src,
			Dst:      dst,
			ReadOnly: ro,
		})
	}

	return mounts, nil
}

func generateJailID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "core"
	}
	return hex.EncodeToString(b)
}

// RunJail handles the `void jail` subcommand
func RunJail(args []string) error {
	fs := flag.NewFlagSet("jail", flag.ContinueOnError)

	var (
		bindFlag   string
		libDirFlag string
		sizeFlag   string
		userFlag   string
		exportFlag bool
	)

	fs.StringVar(&bindFlag, "bind", "", "Bind mount mappings: <src>:<dst>[:ro],...")
	fs.StringVar(&libDirFlag, "lib", "", "Path to collected libraries directory (required)")
	fs.StringVar(&sizeFlag, "size", "64M", "Size of volatile tmpfs RAM filesystem (e.g. 64M, 256M, 512M, 1G)")
	fs.StringVar(&userFlag, "user", "", "User or UID[:GID] to execute target binary inside jail (e.g. postgres, 969:969)")
	fs.StringVar(&userFlag, "u", "", "User or UID[:GID] to execute target binary inside jail (shorthand)")
	fs.BoolVar(&exportFlag, "export", false, "Output the generated shell script to stdout without executing")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("missing target executable binary\nUsage: void jail [-size <size>] -lib <dir> [-bind <mappings>] [-u <user>] [--export] <binary...> [-- <args...>]")
	}

	// '--' でバイナリリストと実行時引数を分離
	var rawBins []string
	var execArgs []string

	dashIndex := -1
	for i, arg := range remaining {
		if arg == "--" {
			dashIndex = i
			break
		}
	}

	if dashIndex != -1 {
		rawBins = remaining[:dashIndex]
		execArgs = remaining[dashIndex+1:]
	} else {
		rawBins = []string{remaining[0]}
		execArgs = remaining[1:]
	}

	if len(rawBins) == 0 {
		return fmt.Errorf("no executable binaries specified before '--'")
	}

	var absBins []string
	for _, b := range rawBins {
		target, err := exec.LookPath(b)
		if err != nil {
			return fmt.Errorf("target executable '%s' not found: %w", b, err)
		}
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("failed to determine absolute path for '%s': %w", target, err)
		}
		absBins = append(absBins, absTarget)
	}

	var absLibDir string
	if libDirFlag != "" {
		var err error
		absLibDir, err = filepath.Abs(libDirFlag)
		if err != nil {
			return fmt.Errorf("failed to determine absolute path for lib dir '%s': %w", libDirFlag, err)
		}
		if _, err := os.Stat(absLibDir); os.IsNotExist(err) {
			return fmt.Errorf("specified lib directory does not exist: %s", absLibDir)
		}
	}

	mounts, err := parseBindFlags(bindFlag)
	if err != nil {
		return fmt.Errorf("bind mount error: %w", err)
	}

	data := JailTemplateData{
		JailID:     generateJailID(),
		Size:       sizeFlag,
		LibDir:     absLibDir,
		User:       userFlag,
		BindMounts: mounts,
		ExecBins:   absBins,
		TargetBin:  filepath.Base(absBins[0]),
		ExecArgs:   execArgs,
	}

	tmpl, err := template.New("jail").Parse(jailScriptTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse jail script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render jail script template: %w", err)
	}

	renderedScript := buf.String()

	if exportFlag {
		fmt.Print(renderedScript)
		return nil
	}

	tmpScript, err := os.CreateTemp("", "void-jail-runner-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temporary runner script: %w", err)
	}
	defer os.Remove(tmpScript.Name())

	if _, err := tmpScript.WriteString(renderedScript); err != nil {
		tmpScript.Close()
		return fmt.Errorf("failed to write runner script: %w", err)
	}
	if err := tmpScript.Chmod(0755); err != nil {
		tmpScript.Close()
		return fmt.Errorf("failed to make runner script executable: %w", err)
	}
	tmpScript.Close()

	cmd := exec.Command("/bin/bash", tmpScript.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Wait()
}
