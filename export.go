package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func RunExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	libDir := fs.String("lib", "", "Path to collected libraries directory (required)")
	bindStr := fs.String("bind", "", "Bind mount mappings: <src>:<dst>[:ro],...")
	output := fs.String("output", "", "Output path (.tar, .tar.gz, or directory) (required)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	posArgs := fs.Args()
	if len(posArgs) < 1 {
		return fmt.Errorf("target executable required\nUsage: void export [--bind=...] --lib <lib dir> --output <tarball|dir> <exec>")
	}
	if *libDir == "" || *output == "" {
		return fmt.Errorf("--lib and --output flags are required")
	}

	execBin, err := filepath.Abs(posArgs[0])
	if err != nil {
		return fmt.Errorf("failed to resolve target binary: %w", err)
	}

	absLibDir, err := filepath.Abs(*libDir)
	if err != nil {
		return fmt.Errorf("failed to resolve lib dir: %w", err)
	}

	mounts, err := parseBindFlags(*bindStr)
	if err != nil {
		return err
	}

	isArchive := strings.HasSuffix(*output, ".tar") || strings.HasSuffix(*output, ".tar.gz") || strings.HasSuffix(*output, ".tgz")

	var stagingDir string
	if isArchive {
		stagingDir, err = os.MkdirTemp("", "void-export-*")
		if err != nil {
			return fmt.Errorf("failed to create staging dir: %w", err)
		}
		defer os.RemoveAll(stagingDir)
	} else {
		stagingDir = *output
		if err := os.MkdirAll(stagingDir, 0755); err != nil {
			return fmt.Errorf("failed to create output dir: %w", err)
		}
	}

	fmt.Printf("[+] Assembling standalone rootfs in: %s\n", stagingDir)

	// 1. スケルトンディレクトリ作成
	dirs := []string{"bin", "dev", "proc", "etc", "tmp"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(stagingDir, d), 0755); err != nil {
			return err
		}
	}

	// 2. resolv.conf コピー
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		_ = copyFile("/etc/resolv.conf", filepath.Join(stagingDir, "etc/resolv.conf"))
	}

	// 3. ライブラリ群の取り込み
	if err := copyDirRecursive(absLibDir, stagingDir); err != nil {
		return fmt.Errorf("failed to copy libraries: %w", err)
	}

	// UsrMerge シンボリックリンクの再現 (/lib64 -> usr/lib64 など)
	for _, dir := range []string{"lib", "lib64"} {
		usrSubdir := filepath.Join(stagingDir, "usr", dir)
		rootSubdir := filepath.Join(stagingDir, dir)
		if _, err := os.Stat(usrSubdir); err == nil {
			if _, err := os.Lstat(rootSubdir); os.IsNotExist(err) {
				_ = os.Symlink(filepath.Join("usr", dir), rootSubdir)
			}
		}
	}

	// 4. 実行バイナリの配置 (/bin/app)
	destBin := filepath.Join(stagingDir, "bin", "app")
	if err := copyFile(execBin, destBin); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}
	_ = os.Chmod(destBin, 0755)

	// 5. バインド対象ファイル/ディレクトリの取り込み
	for _, b := range mounts {
		dstPath := filepath.Join(stagingDir, b.Dst)
		fi, err := os.Stat(b.Src)
		if err != nil {
			return fmt.Errorf("bind source not found: %s", b.Src)
		}

		if fi.IsDir() {
			if err := copyDirRecursive(b.Src, dstPath); err != nil {
				return fmt.Errorf("failed to copy bind dir %s: %w", b.Src, err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return err
			}
			if err := copyFile(b.Src, dstPath); err != nil {
				return fmt.Errorf("failed to copy bind file %s: %w", b.Src, err)
			}
		}
		fmt.Printf("  -> [EMBEDDED] %s -> %s\n", b.Src, b.Dst)
	}

	// 6. アーカイブ化 (指定されている場合)
	if isArchive {
		fmt.Printf("[*] Packaging archive into: %s\n", *output)
		if err := createTarArchive(stagingDir, *output); err != nil {
			return fmt.Errorf("failed to create archive: %w", err)
		}
	}

	fmt.Printf("[+] Export completed successfully: %s\n", *output)
	return nil
}

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)

		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(linkDest, target)
		}

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}

		return copyFile(path, target)
	})
}

func createTarArchive(srcDir, tarPath string) error {
	outFile, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	var tw *tar.Writer
	if strings.HasSuffix(tarPath, ".gz") || strings.HasSuffix(tarPath, ".tgz") {
		gw := gzip.NewWriter(outFile)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(outFile)
	}
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return nil
		}

		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = rel

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
}
