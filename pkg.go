package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const pacmanCacheDir = "/var/cache/pacman/pkg"

// extractPackageName parses the canonical Arch package name from an archive filename
// e.g., "postgresql-libs-18.6-2-x86_64.pkg.tar.zst" -> "postgresql-libs"
//       "postgresql-18.6-2-x86_64.pkg.tar.zst"      -> "postgresql"
func extractPackageName(filename string) string {
	base := filepath.Base(filename)
	for _, ext := range []string{".pkg.tar.zst", ".pkg.tar.xz", ".pkg.tar.gz", ".pkg.tar"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	parts := strings.Split(base, "-")
	// Arch Linux パッケージ命名規則: <pkgname>-<pkgver>-<pkgrel>-<arch>
	// pkgver, pkgrel, arch はハイフンを含まないため、末尾から3要素を除いた残りが完全なパッケージ名
	if len(parts) >= 4 {
		return strings.Join(parts[:len(parts)-3], "-")
	}
	return ""
}

// parseDependencies extracts clean package names from .PKGINFO inside the tar.zst archive
func parseDependencies(pkgPath string) ([]string, error) {
	cmd := exec.Command("tar", "--zstd", "-xOf", pkgPath, ".PKGINFO")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read .PKGINFO from %s: %w", filepath.Base(pkgPath), err)
	}

	var deps []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "depend = ") {
			rawDep := strings.TrimSpace(strings.TrimPrefix(line, "depend = "))
			// バージョン制約やSONAME表記(>=, <=, =, :)を除去して純粋なパッケージ名を取得
			depName := strings.FieldsFunc(rawDep, func(r rune) bool {
				return r == '>' || r == '<' || r == '=' || r == ':'
			})[0]
			deps = append(deps, strings.TrimSpace(depName))
		}
	}
	return deps, nil
}

// findLatestArchive searches multiple directories for matching package archives and selects the newest one
func findLatestArchive(pkgName string, searchDirs ...string) string {
	var candidates []string

	for _, dir := range searchDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		pattern := filepath.Join(dir, fmt.Sprintf("%s-*.pkg.tar.zst", pkgName))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if strings.HasSuffix(m, ".sig") {
				continue
			}
			base := filepath.Base(m)
			// 完全一致判定: "postgresql" に対して "postgresql-libs" などを確実に弾く
			if extractPackageName(base) == pkgName {
				candidates = append(candidates, m)
			}
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	var latestPath string
	var latestTime time.Time

	for _, c := range candidates {
		info, err := os.Stat(c)
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestPath = c
		}
	}

	return latestPath
}

// downloadLatestPkg downloads the newest package into the pacman cache
func downloadLatestPkg(pkgName string) error {
	fmt.Printf("[*] Downloading latest package via pacman: %s\n", pkgName)
	cmd := exec.Command("pacman", "-Sw", "--cachedir", pacmanCacheDir, "--noconfirm", pkgName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pacman download failed for '%s': %w\nOutput: %s", pkgName, err, string(out))
	}
	return nil
}

// locateOrDownloadPkg resolves archive path based on operational mode
func locateOrDownloadPkg(pkgName, targetDir string, forceLatest bool) (string, error) {
	if forceLatest {
		// 最新優先モード: 常に pacman 公式リポジトリからキャッシュを同期
		if err := downloadLatestPkg(pkgName); err != nil {
			fmt.Printf("[-] Warning: Failed to fetch latest '%s': %v\n", pkgName, err)
		}
		candidate := findLatestArchive(pkgName, pacmanCacheDir, targetDir)
		if candidate != "" {
			return candidate, nil
		}
		return "", fmt.Errorf("failed to locate downloaded archive for: %s", pkgName)
	}

	// 通常モード: ローカルキャッシュを優先探索
	candidate := findLatestArchive(pkgName, targetDir, pacmanCacheDir)
	if candidate != "" {
		return candidate, nil
	}

	// キャッシュに存在しない場合はフォールバックとしてダウンロード
	fmt.Printf("[!] Package not found in cache. Attempting fallback download: %s\n", pkgName)
	if err := downloadLatestPkg(pkgName); err != nil {
		return "", err
	}

	candidate = findLatestArchive(pkgName, pacmanCacheDir, targetDir)
	if candidate != "" {
		return candidate, nil
	}
	return "", fmt.Errorf("package archive not found for: %s", pkgName)
}

// extractPkg extracts the tar.zst archive into outDir excluding packaging metadata
func extractPkg(pkgPath, outDir string) error {
	cmd := exec.Command("tar",
		"--zstd", "-xf", pkgPath, "--overwrite",
		"-C", outDir,
		"--exclude=.PKGINFO",
		"--exclude=.BUILDINFO",
		"--exclude=.MTREE",
		"--exclude=.INSTALL",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar extract failed for %s: %w\nOutput: %s", filepath.Base(pkgPath), err, string(out))
	}
	return nil
}

// unpackRecursive unpacks the specified package and recursively resolves all its dependencies
func unpackRecursive(pkgPath, outDir, baseDir string, visited map[string]bool, forceLatest bool) error {
	pkgFile := filepath.Base(pkgPath)
	if visited[pkgFile] {
		return nil
	}
	// 展開対象としてマーク
	visited[pkgFile] = true

	fmt.Printf("[*] Unpacking package: %s\n", pkgFile)
	if err := extractPkg(pkgPath, outDir); err != nil {
		return err
	}

	deps, err := parseDependencies(pkgPath)
	if err != nil {
		fmt.Printf("[-] Warning: %v\n", err)
		return nil
	}

	for _, dep := range deps {
		// 依存パッケージ名がすでに探索済みならスキップ
		if visited[dep] {
			continue
		}

		depPkgPath, err := locateOrDownloadPkg(dep, baseDir, forceLatest)
		if err != nil {
			fmt.Printf("[-] Warning: Failed to resolve dependency '%s': %v\n", dep, err)
			continue
		}

		// 探索済みとしてマークしてから再帰呼び出し
		visited[dep] = true
		if err := unpackRecursive(depPkgPath, outDir, baseDir, visited, forceLatest); err != nil {
			return err
		}
	}

	return nil
}

// RunPkg handles `void pkg [--latest] <pkg-name|archive.pkg.tar.zst> <output-rootfs-dir>`
func RunPkg(args []string) error {
	fs := flag.NewFlagSet("pkg", flag.ContinueOnError)
	forceLatest := fs.Bool("latest", false, "download and resolve all packages to latest official versions")
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) < 2 {
		return fmt.Errorf("missing arguments\nUsage:\n  void pkg --latest <package-name> <output-rootfs-dir>\n  void pkg <archive.pkg.tar.zst> <output-rootfs-dir>")
	}

	target := remaining[0]
	outDir, err := filepath.Abs(remaining[1])
	if err != nil {
		return fmt.Errorf("invalid output directory: %w", err)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create rootfs directory: %w", err)
	}

	var primaryPkgPath string
	var pkgDir string

	isArchiveFile := strings.HasSuffix(target, ".pkg.tar.zst") || strings.HasSuffix(target, ".tar.zst")
	if !*forceLatest && isArchiveFile {
		// アーカイブ直接指定モード（既存キャッシュ参照・互換性非保証）
		absPath, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("invalid package path: %w", err)
		}
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return fmt.Errorf("specified package file not found: %s", absPath)
		}
		primaryPkgPath = absPath
		pkgDir = filepath.Dir(absPath)

		fmt.Println("[!] NOTICE: Archive mode specified.")
		fmt.Println("[!] Dependencies will be looked up from cache. Version compatibility is NOT guaranteed.")
		fmt.Printf("[*] Primary target archive: %s\n", filepath.Base(primaryPkgPath))
	} else {
		// パッケージ名指定モード（または --latest 付き）: 公式リポジトリから最新版を取得
		pkgName := target
		if isArchiveFile {
			if extPkg := extractPackageName(target); extPkg != "" {
				pkgName = extPkg
			}
		}

		fmt.Printf("[*] Mode: LATEST (Fetching newest packages for '%s' from official repo)\n", pkgName)
		if err := downloadLatestPkg(pkgName); err != nil {
			return fmt.Errorf("failed to download latest primary package: %w", err)
		}

		primaryPkgPath = findLatestArchive(pkgName, pacmanCacheDir)
		if primaryPkgPath == "" {
			return fmt.Errorf("could not find downloaded primary package archive for: %s", pkgName)
		}
		pkgDir = pacmanCacheDir
		*forceLatest = true
		fmt.Printf("[*] Primary target package: %s\n", filepath.Base(primaryPkgPath))
	}

	fmt.Printf("[*] Target rootfs: %s\n", outDir)

	// 1. 再帰的に依存関係を解決して展開（primary パッケージ自体を確実に展開）
	visited := make(map[string]bool)
	if err := unpackRecursive(primaryPkgPath, outDir, pkgDir, visited, *forceLatest); err != nil {
		return fmt.Errorf("recursive packaging failed: %w", err)
	}
	fmt.Println("[+] All dependent packages merged into rootfs successfully.")

	// 2. rootfs 内の lib ディレクトリを作成
	targetLibDir := outDir
	if err := os.MkdirAll(targetLibDir, 0755); err != nil {
		return fmt.Errorf("failed to create lib dir: %w", err)
	}

	// 3. 展開されたすべてのバイナリおよび .so ファイルから ELF 依存関係を収集
	fmt.Println("[*] Scanning projected binaries for ELF dependencies (void ldd)...")
	var scanTargets []string

	err = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(path, "/bin/") || strings.Contains(filepath.Base(path), ".so") {
			if info.Mode()&0111 != 0 || strings.Contains(filepath.Base(path), ".so") {
				scanTargets = append(scanTargets, path)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan binaries: %w", err)
	}

	fmt.Printf("[*] Running recursive ldd resolver on %d ELFs...\n", len(scanTargets))
	for _, target := range scanTargets {
		_ = ResolveAndCollectLibraries(target, targetLibDir)
	}

	// 4. パーミッション調整 (降格実行ユーザーの読み取り・実行権限を開放)
	_ = filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || info.Mode()&0111 != 0 {
			_ = os.Chmod(path, 0755)
		} else {
			_ = os.Chmod(path, 0644)
		}
		return nil
	})

	fmt.Println("[+] Rootfs packaging complete with all dependencies and runtime libraries.")
	return nil
}