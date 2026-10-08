package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*
var embeddedAssets embed.FS

type ScaffoldConfig struct {
	TargetDir  string
	ModuleName string
	IsGo       bool
}

// RunScaffold は void scaffold サブコマンドのエントリポイント
func RunScaffold(args []string) error {
	fsCmd := flag.NewFlagSet("scaffold", flag.ContinueOnError)
	goFlag := fsCmd.Bool("go", false, "Scaffold Go stack (Gin + HTMX + CSRF/XSS) instead of default OCaml")

	if err := fsCmd.Parse(args); err != nil {
		return err
	}

	remaining := fsCmd.Args()
	if len(remaining) < 1 {
		return errors.New("usage: void scaffold [--go] <app dir>")
	}

	targetDir := remaining[0]
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}

	moduleName := filepath.Base(absTarget)

	// SPEC.md に定義された Void-9 の標準骨格ディレクトリ群を作成
	baseDirs := []string{
		filepath.Join(absTarget, "bin"),
		filepath.Join(absTarget, "lib"),
		filepath.Join(absTarget, "rootfs", "dev"),
		filepath.Join(absTarget, "rootfs", "proc"),
		filepath.Join(absTarget, "rootfs", "etc"),
		filepath.Join(absTarget, "rootfs", "tmp"),
	}

	for _, dir := range baseDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create base directory %s: %w", dir, err)
		}
	}

	// テンプレートパスの選択（デフォルトは OCaml、--go で Go）
	templateSubdir := "templates/ocaml"
	stackName := "OCaml (Dune + Dream)"
	if *goFlag {
		templateSubdir = "templates/go"
		stackName = "Go (Gin + HTMX + SQLite3 + CSRF/XSS)"
	}

	fmt.Printf("[*] Scaffolding Void environment [%s] into: %s\n", stackName, targetDir)

	subFS, err := fs.Sub(embeddedAssets, templateSubdir)
	if err != nil {
		// テンプレートディレクトリがまだ存在しない場合へのフォールバック対応
		// (移行途中で templates 直下に Go ファイルが残っている場合)
		if *goFlag {
			subFS, err = fs.Sub(embeddedAssets, "templates")
			if err != nil {
				return fmt.Errorf("failed to load template stack: %w", err)
			}
		} else {
			return fmt.Errorf("ocaml templates not found in %s: %w", templateSubdir, err)
		}
	}

	templateVars := map[string]string{
		"MODULE_NAME": moduleName,
	}

	if err := renderTemplateTree(subFS, absTarget, templateVars); err != nil {
		return fmt.Errorf("failed to render scaffold template: %w", err)
	}

	fmt.Printf("[+] Successfully scaffolded %s skeleton in %s\n", stackName, targetDir)
	return nil
}

func renderTemplateTree(srcFS fs.FS, destRoot string, vars map[string]string) error {
	return fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		// ocaml や go の親ディレクトリ自体はスキップ
		cleanPath := strings.TrimPrefix(path, "templates/")

		relPath := strings.TrimSuffix(cleanPath, ".tmpl")
		destPath := filepath.Join(destRoot, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		content, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// .tmpl 拡張子のファイルは変数置換して保存
		if strings.HasSuffix(path, ".tmpl") {
			tmpl, err := template.New(filepath.Base(path)).Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse template %s: %w", path, err)
			}

			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer f.Close()

			return tmpl.Execute(f, vars)
		}

		// 静的バイナリファイルや CSS/JS などの通常ファイルはそのままコピー
		return os.WriteFile(destPath, content, 0644)
	})
}
