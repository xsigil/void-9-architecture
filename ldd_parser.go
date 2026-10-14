package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// TokenType トークン種別
type TokenType int

const (
	TokenEOF     TokenType = iota
	TokenIdent             // ファイル名やパス (例: libc.so.6, /lib/x86_64-linux-gnu/libc.so.6)
	TokenArrow             // "=>"
	TokenAddress           // "(0x0000...)"
)

type Token struct {
	Type  TokenType
	Value string
}

// Lexer ldd出力用字句解析器
type Lexer struct {
	input []rune
	pos   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: []rune(input), pos: 0}
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF}
	}

	ch := l.input[l.pos]

	// "=>" の判定
	if ch == '=' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '>' {
		l.pos += 2
		return Token{Type: TokenArrow, Value: "=>"}
	}

	// アドレス "(0x...)" の判定
	if ch == '(' {
		start := l.pos
		for l.pos < len(l.input) && l.input[l.pos] != ')' && l.input[l.pos] != '\n' {
			l.pos++
		}
		if l.pos < len(l.input) && l.input[l.pos] == ')' {
			l.pos++
		}
		return Token{Type: TokenAddress, Value: string(l.input[start:l.pos])}
	}

	// パスやライブラリ名 (空白・括弧・イコール以外)
	start := l.pos
	for l.pos < len(l.input) && !unicode.IsSpace(l.input[l.pos]) && l.input[l.pos] != '(' && l.input[l.pos] != '=' {
		l.pos++
	}
	val := string(l.input[start:l.pos])
	return Token{Type: TokenIdent, Value: val}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\r') {
		l.pos++
	}
}

// ParsedLibrary 抽出されたライブラリ情報
type ParsedLibrary struct {
	Name     string // soname またはベース名
	RealPath string // ホスト上の絶対パス
}

// Parser ldd出力行の構文解析器
type Parser struct {
	lexer        *Lexer
	currentToken Token
}

func NewParser(line string) *Parser {
	lexer := NewLexer(line)
	p := &Parser{lexer: lexer}
	p.currentToken = p.lexer.NextToken()
	return p
}

func (p *Parser) consume(t TokenType) (string, error) {
	if p.currentToken.Type != t {
		return "", fmt.Errorf("unexpected token %v, expected %v", p.currentToken.Type, t)
	}
	val := p.currentToken.Value
	p.currentToken = p.lexer.NextToken()
	return val, nil
}

// ParseLine 1行分のldd出力を解析
func (p *Parser) ParseLine() (*ParsedLibrary, error) {
	if p.currentToken.Type == TokenEOF {
		return nil, nil
	}

	firstIdent, err := p.consume(TokenIdent)
	if err != nil {
		return nil, err
	}

	// パターン1: "linux-vdso.so.1 (0x...)" -> 実体パスなし (カーネルVDSO)
	if p.currentToken.Type == TokenAddress {
		if strings.HasPrefix(firstIdent, "/") {
			// パターン2: "/lib64/ld-linux-x86-64.so.2 (0x...)" -> インタープリタ直指定
			return &ParsedLibrary{
				Name:     filepath.Base(firstIdent),
				RealPath: firstIdent,
			}, nil
		}
		// vdso 等の仮想エントリはスキップ
		return nil, nil
	}

	// パターン3: "libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x...)"
	if p.currentToken.Type == TokenArrow {
		_, _ = p.consume(TokenArrow)
		realPath, err := p.consume(TokenIdent)
		if err != nil {
			return nil, err
		}
		if p.currentToken.Type == TokenAddress {
			_, _ = p.consume(TokenAddress)
		}

		// "not found" などの例外ガード
		if !strings.HasPrefix(realPath, "/") {
			return nil, fmt.Errorf("unresolved library target: %s", realPath)
		}

		return &ParsedLibrary{
			Name:     firstIdent,
			RealPath: realPath,
		}, nil
	}

	return nil, nil
}

// ResolveAndCollectLibraries lddを実行し、解析した共有ライブラリをコピーする
func ResolveAndCollectLibraries(execPath, targetLibDir string) error {
	absExec, err := filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve exec path: %w", err)
	}

	cmd := exec.Command("ldd", absExec)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ldd failed on %s: %w", absExec, err)
	}

	lines := strings.Split(string(out), "\n")
	collected := make(map[string]bool)
	hasGlibc := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		parser := NewParser(trimmed)
		lib, err := parser.ParseLine()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [!] parse warning on '%s': %v\n", trimmed, err)
			continue
		}
		if lib == nil || lib.RealPath == "" {
			continue
		}

		if strings.Contains(filepath.Base(lib.RealPath), "libc.so") {
			hasGlibc = true
		}

		if collected[lib.RealPath] {
			continue
		}
		collected[lib.RealPath] = true

		if err := copyLibraryIncremental(lib, targetLibDir); err != nil {
			return fmt.Errorf("failed to copy library %s: %w", lib.RealPath, err)
		}
	}

	// glibc を使用している場合、実行時の NSS (Name Service Switch / getpwuid) 用共有ライブラリを自動収集
	if hasGlibc {
		patterns := []string{
			"/usr/lib*/libnss_files.so*",
			"/usr/lib*/libnss_compat.so*",
			"/lib*/libnss_files.so*",
			"/lib*/libnss_compat.so*",
			"/lib/*-linux-gnu/libnss_files.so*",
			"/lib/*-linux-gnu/libnss_compat.so*",
		}

		for _, pat := range patterns {
			matches, _ := filepath.Glob(pat)
			for _, m := range matches {
				realPath, err := filepath.Abs(m)
				if err != nil || collected[realPath] {
					continue
				}
				collected[realPath] = true
				nssLib := &ParsedLibrary{
					Name:     filepath.Base(realPath),
					RealPath: realPath,
				}
				if err := copyLibraryIncremental(nssLib, targetLibDir); err != nil {
					fmt.Fprintf(os.Stderr, "  [!] warning: failed to copy nss module %s: %v\n", realPath, err)
				}
			}
		}
	}

	return nil
}

// copyLibraryIncremental 実体ファイルとシンボリックリンク関係を保持してインクリメンタルコピー
func copyLibraryIncremental(lib *ParsedLibrary, targetLibDir string) error {
	destRel := strings.TrimPrefix(lib.RealPath, "/")
	destPath := filepath.Join(targetLibDir, destRel)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	if info, err := os.Lstat(destPath); err == nil {
		if !info.IsDir() {
			return nil
		}
	}

	fi, err := os.Lstat(lib.RealPath)
	if err != nil {
		return err
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(lib.RealPath)
		if err != nil {
			return err
		}

		var resolvedTarget string
		if filepath.IsAbs(linkTarget) {
			resolvedTarget = linkTarget
		} else {
			resolvedTarget = filepath.Clean(filepath.Join(filepath.Dir(lib.RealPath), linkTarget))
		}

		subLib := &ParsedLibrary{
			Name:     filepath.Base(resolvedTarget),
			RealPath: resolvedTarget,
		}
		if err := copyLibraryIncremental(subLib, targetLibDir); err != nil {
			return err
		}

		_ = os.Remove(destPath)
		if err := os.Symlink(linkTarget, destPath); err != nil {
			return err
		}
		fmt.Printf("  -> [SYMLINK] %s -> %s\n", destRel, linkTarget)
		return nil
	}

	if err := copyFile(lib.RealPath, destPath); err != nil {
		return err
	}

	fmt.Printf("  -> [LIB COPIED] %s\n", destRel)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
