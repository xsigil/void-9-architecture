package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

type BindMount struct {
	Src      string
	Dst      string
	ReadOnly bool
}

type JailConfig struct {
	JailID     string
	TmpfsSize  string
	LibDir     string
	ExecBin    string
	ExecArgs   []string
	BindMounts []BindMount
}

func parseBindFlags(bindStr string) ([]BindMount, error) {
	if bindStr == "" {
		return nil, nil
	}
	var mounts []BindMount
	entries := strings.Split(bindStr, ",")
	for _, entry := range entries {
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("invalid bind mount format '%s', expected <src>:<dst>[:ro]", entry)
		}
		src, err := filepath.Abs(parts[0])
		if err != nil {
			return nil, err
		}
		dst := strings.TrimPrefix(parts[1], "/")
		ro := false
		if len(parts) == 3 {
			if parts[2] != "ro" {
				return nil, fmt.Errorf("invalid bind mount flag '%s', expected 'ro'", parts[2])
			}
			ro = true
		}
		mounts = append(mounts, BindMount{
			Src:      src,
			Dst:      dst,
			ReadOnly: ro,
		})
	}
	return mounts, nil
}

func RunJail(args []string) error {
	fs := flag.NewFlagSet("jail", flag.ExitOnError)
	libDir := fs.String("lib", "", "Path to collected libraries directory (required)")
	bindStr := fs.String("bind", "", "Bind mount mappings: <src>:<dst>[:ro],...")
	tmpfsSize := fs.String("size", "64M", "Size of volatile tmpfs RAM filesystem")

	if err := fs.Parse(args); err != nil {
		return err
	}

	posArgs := fs.Args()
	if len(posArgs) < 1 {
		return fmt.Errorf("target executable required\nUsage: void jail [--bind=<src>:<dst>[:ro],...] [--lib <lib dir>] <exec> [args...]")
	}

	execBin, err := filepath.Abs(posArgs[0])
	if err != nil {
		return fmt.Errorf("failed to resolve target binary: %w", err)
	}

	var absLibDir string
	if *libDir != "" {
		absLibDir, err = filepath.Abs(*libDir)
		if err != nil {
			return fmt.Errorf("failed to resolve lib dir: %w", err)
		}
	}

	mounts, err := parseBindFlags(*bindStr)
	if err != nil {
		return err
	}

	cfg := JailConfig{
		JailID:     fmt.Sprintf("%d", os.Getpid()),
		TmpfsSize:  *tmpfsSize,
		LibDir:     absLibDir,
		ExecBin:    execBin,
		ExecArgs:   posArgs[1:],
		BindMounts: mounts,
	}

	// 埋め込みファイルシステムからテンプレートを取得
	tmplContent, err := embeddedAssets.ReadFile("templates/jail.sh.tmpl")
	if err != nil {
		return fmt.Errorf("failed to read jail template: %w", err)
	}

	tmpl, err := template.New("jail").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse jail template: %w", err)
	}

	var scriptBuf bytes.Buffer
	if err := tmpl.Execute(&scriptBuf, cfg); err != nil {
		return fmt.Errorf("failed to render jail script: %w", err)
	}

	cmd := exec.Command("bash", "-c", scriptBuf.String())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
