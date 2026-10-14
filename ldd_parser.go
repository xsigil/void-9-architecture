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

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenArrow
	TokenAddress
)

type Token struct {
	Type  TokenType
	Value string
}

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

	if ch == '=' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '>' {
		l.pos += 2
		return Token{Type: TokenArrow, Value: "=>"}
	}

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

type ParsedLibrary struct {
	Name     string
	RealPath string
}

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

func (p *Parser) ParseLine() (*ParsedLibrary, error) {
	if p.currentToken.Type == TokenEOF {
		return nil, nil
	}

	firstIdent, err := p.consume(TokenIdent)
	if err != nil {
		return nil, err
	}

	if p.currentToken.Type == TokenAddress {
		if strings.HasPrefix(firstIdent, "/") {
			return &ParsedLibrary{
				Name:     filepath.Base(firstIdent),
				RealPath: firstIdent,
			}, nil
		}
		return nil, nil
	}

	if p.currentToken.Type == TokenArrow {
		_, _ = p.consume(TokenArrow)
		realPath, err := p.consume(TokenIdent)
		if err != nil {
			return nil, err
		}
		if p.currentToken.Type == TokenAddress {
			_, _ = p.consume(TokenAddress)
		}

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

	// ld-linux を確実に /usr/lib 直下へコピー
	ldMatches, _ := filepath.Glob("/usr/lib/ld-linux*.so*")
	for _, m := range ldMatches {
		if collected[m] {
			continue
		}
		collected[m] = true
		_ = copyLibraryIncremental(&ParsedLibrary{
			Name:     filepath.Base(m),
			RealPath: m,
		}, targetLibDir)
	}

	return nil
}

func copyLibraryIncremental(lib *ParsedLibrary, targetLibDir string) error {
	// ホストの実パスからシンボリックリンクを解決
	evalPath, err := filepath.EvalSymlinks(lib.RealPath)
	if err != nil {
		evalPath = lib.RealPath
	}

	// すべてのライブラリ実体を targetLibDir/usr/lib 直下に配置
	baseName := filepath.Base(evalPath)
	destDir := filepath.Join(targetLibDir, "usr", "lib")
	destPath := filepath.Join(destDir, baseName)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// 実体のコピー
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		if err := copyFile(evalPath, destPath); err != nil {
			return err
		}
		fmt.Printf("  -> [LIB COPIED] usr/lib/%s\n", baseName)
	}

	// 元の参照名が実体名と異なる場合は usr/lib 配下にシンボリックリンクを作成
	origName := filepath.Base(lib.RealPath)
	if origName != baseName {
		symlinkPath := filepath.Join(destDir, origName)
		_ = os.Remove(symlinkPath)
		_ = os.Symlink(baseName, symlinkPath)
		fmt.Printf("  -> [SYMLINK] usr/lib/%s -> %s\n", origName, baseName)
	}

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
