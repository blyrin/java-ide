package main

import (
	"unicode"
	"unicode/utf8"
)

type jTokType int

const (
	jTokEOF jTokType = iota
	jTokError
	jTokIdent
	jTokKeyword
	jTokLiteral
	jTokSymbol
	jTokComment
)

func (t jTokType) String() string {
	switch t {
	case jTokEOF:
		return "EOF"
	case jTokError:
		return "Error"
	case jTokIdent:
		return "Identifier"
	case jTokKeyword:
		return "Keyword"
	case jTokLiteral:
		return "Literal"
	case jTokSymbol:
		return "Symbol"
	case jTokComment:
		return "Comment"
	default:
		return "Unknown"
	}
}

type jTok struct {
	Type    jTokType
	Literal string
	Range   jRange
	From    int
	To      int
}

func (t jTok) isSym(s string) bool { return t.Type == jTokSymbol && t.Literal == s }
func (t jTok) isKw(s string) bool  { return t.Type == jTokKeyword && t.Literal == s }
func (t jTok) isWord(s string) bool {
	return (t.Type == jTokIdent || t.Type == jTokKeyword) && t.Literal == s
}

type jLexer struct {
	src     []byte
	pos     int
	readPos int
	ch      rune
	line    int
	col     int
}

func newJLexer(src string) *jLexer {
	l := &jLexer{src: []byte(src), line: 1}
	l.read()
	return l
}

func (l *jLexer) read() {
	if l.readPos >= len(l.src) {
		l.ch = 0
		l.pos = l.readPos
		l.readPos++
		return
	}
	r, size := utf8.DecodeRune(l.src[l.readPos:])
	l.ch = r
	l.pos = l.readPos
	l.readPos += size
	if r == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col += size
	}
}

func (l *jLexer) peek() rune {
	if l.readPos >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRune(l.src[l.readPos:])
	return r
}

func (l *jLexer) peekByte(off int) byte {
	i := l.readPos + off
	if i < 0 || i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

func (l *jLexer) byteAt(i int) byte {
	if i < 0 || i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

func (l *jLexer) hasWordAt(i int, w string) bool {
	if i < 0 || i+len(w) > len(l.src) {
		return false
	}
	return string(l.src[i:i+len(w)]) == w
}

func (l *jLexer) here() jPos { return jPos{Line: l.line, Col: l.col} }

func (l *jLexer) tok(t jTokType, lit string, from int, start jPos) jTok {
	return jTok{Type: t, Literal: lit, Range: jRange{Start: start, End: l.here()}, From: from, To: l.pos}
}

func (l *jLexer) skipSpace() {
	for l.ch != 0 && (unicode.IsSpace(l.ch) || l.ch == jBOM) {
		l.read()
	}
}

func (l *jLexer) nextTok() jTok {
	l.skipSpace()
	from := l.pos
	start := l.here()
	if l.ch == 0 {
		return l.tok(jTokEOF, "", from, start)
	}
	switch {
	case l.ch == '/' && l.peek() == '/':
		return l.scanLineComment(from, start)
	case l.ch == '/' && l.peek() == '*':
		return l.scanBlockComment(from, start)
	case l.ch == '"':
		if l.peek() == '"' && l.peekByte(1) == '"' {
			return l.scanTextBlock(from, start)
		}
		return l.scanString(from, start)
	case l.ch == '\'':
		return l.scanChar(from, start)
	}
	if isJDigit(l.ch) || (l.ch == '.' && isJDigit(l.peek())) {
		return l.scanNumber(from, start)
	}
	if isJIdentStart(l.ch) {
		lit := l.scanIdent()
		switch {
		case isJKeyword(lit):
			return l.tok(jTokKeyword, lit, from, start)
		case lit == "true" || lit == "false" || lit == "null":
			return l.tok(jTokLiteral, lit, from, start)
		default:
			return l.tok(jTokIdent, lit, from, start)
		}
	}
	if op := l.scanOperator(); op != "" {
		return l.tok(jTokSymbol, op, from, start)
	}
	bad := l.ch
	l.read()
	return l.tok(jTokError, string(bad), from, start)
}

func (l *jLexer) scanLineComment(from int, start jPos) jTok {
	for l.ch != 0 && l.ch != '\n' && l.ch != '\r' {
		l.read()
	}
	return l.tok(jTokComment, string(l.src[from:l.pos]), from, start)
}

func (l *jLexer) scanBlockComment(from int, start jPos) jTok {
	l.read()
	l.read()
	for l.ch != 0 {
		if l.ch == '*' && l.peek() == '/' {
			l.read()
			l.read()
			break
		}
		l.read()
	}
	return l.tok(jTokComment, string(l.src[from:l.pos]), from, start)
}

func (l *jLexer) scanString(from int, start jPos) jTok {
	l.read()
	for l.ch != 0 && l.ch != '"' && l.ch != '\n' {
		if l.ch == '\\' {
			l.read()
		}
		l.read()
	}
	if l.ch == '"' {
		l.read()
	}
	return l.tok(jTokLiteral, string(l.src[from:l.pos]), from, start)
}

func (l *jLexer) scanChar(from int, start jPos) jTok {
	l.read()
	for l.ch != 0 && l.ch != '\'' && l.ch != '\n' {
		if l.ch == '\\' {
			l.read()
		}
		l.read()
	}
	if l.ch == '\'' {
		l.read()
	}
	return l.tok(jTokLiteral, string(l.src[from:l.pos]), from, start)
}

func (l *jLexer) scanTextBlock(from int, start jPos) jTok {
	l.read()
	l.read()
	l.read()
	for l.ch != 0 {
		if l.ch == '"' && l.peek() == '"' && l.peekByte(1) == '"' {
			l.read()
			l.read()
			l.read()
			break
		}
		if l.ch == '\\' {
			l.read()
		}
		l.read()
	}
	return l.tok(jTokLiteral, string(l.src[from:l.pos]), from, start)
}

func (l *jLexer) scanIdent() string {
	from := l.pos
	for l.ch != 0 && isJIdentPart(l.ch) {
		l.read()
	}
	if string(l.src[from:l.pos]) == "non" && l.ch == '-' &&
		l.hasWordAt(l.pos+1, "sealed") && !isJIdentByte(l.byteAt(l.pos+7)) {
		for range 7 {
			l.read()
		}
	}
	return string(l.src[from:l.pos])
}

func (l *jLexer) scanNumber(from int, start jPos) jTok {
	switch {
	case l.ch == '0' && (l.peek() == 'x' || l.peek() == 'X'):
		l.read()
		l.read()
		for isJHex(l.ch) || l.ch == '_' {
			l.read()
		}
		if l.ch == '.' {
			l.read()
			for isJHex(l.ch) || l.ch == '_' {
				l.read()
			}
		}
		if l.ch == 'p' || l.ch == 'P' {
			l.read()
			if l.ch == '+' || l.ch == '-' {
				l.read()
			}
			for isJDigit(l.ch) {
				l.read()
			}
		}
	case l.ch == '0' && (l.peek() == 'b' || l.peek() == 'B'):
		l.read()
		l.read()
		for l.ch == '0' || l.ch == '1' || l.ch == '_' {
			l.read()
		}
	default:
		for isJDigit(l.ch) || l.ch == '_' {
			l.read()
		}
		if l.ch == '.' && isJDigit(l.peek()) {
			l.read()
			for isJDigit(l.ch) || l.ch == '_' {
				l.read()
			}
		}
		if l.ch == 'e' || l.ch == 'E' {
			switch {
			case isJDigit(l.peek()):
				l.read()
				for isJDigit(l.ch) || l.ch == '_' {
					l.read()
				}
			case (l.peek() == '+' || l.peek() == '-') && isJDigit(rune(l.peekByte(1))):
				l.read()
				l.read()
				for isJDigit(l.ch) || l.ch == '_' {
					l.read()
				}
			}
		}
	}
	for l.ch == 'l' || l.ch == 'L' || l.ch == 'f' || l.ch == 'F' || l.ch == 'd' || l.ch == 'D' {
		l.read()
	}
	return l.tok(jTokLiteral, string(l.src[from:l.pos]), from, start)
}

var jOperators = []string{
	"...", "->", "::", "++", "--", "&&", "||", "==", "!=", "<=",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<",
	"+", "-", "*", "/", "%", "=", "<", ">", "&", "|", "^", "~", "!",
	"?", ":", ";", ",", ".", "@", "(", ")", "[", "]", "{", "}",
}

func (l *jLexer) scanOperator() string {
	for _, op := range jOperators {
		if l.hasWordAt(l.pos, op) {
			for range len(op) {
				l.read()
			}
			return op
		}
	}
	return ""
}

const jBOM = 0xFEFF

func isJDigit(r rune) bool { return r >= '0' && r <= '9' }

func isJHex(r rune) bool {
	return isJDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isJIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_' || r == '$'
}

func isJIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

func isJIdentByte(b byte) bool {
	return b == '_' || b == '$' || isJDigit(rune(b)) ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

var jKeywordSet = map[string]bool{
	"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true,
	"case": true, "catch": true, "char": true, "class": true, "const": true,
	"continue": true, "default": true, "do": true, "double": true, "else": true,
	"enum": true, "extends": true, "final": true, "finally": true, "float": true,
	"for": true, "goto": true, "if": true, "implements": true, "import": true,
	"instanceof": true, "int": true, "interface": true, "long": true, "native": true,
	"new": true, "package": true, "private": true, "protected": true, "public": true,
	"return": true, "short": true, "static": true, "strictfp": true, "super": true,
	"switch": true, "synchronized": true, "this": true, "throw": true, "throws": true,
	"transient": true, "try": true, "void": true, "volatile": true, "while": true,
	"_": true, "non-sealed": true,
}

func isJKeyword(s string) bool { return jKeywordSet[s] }

var jPrimitiveSet = map[string]bool{
	"boolean": true, "byte": true, "char": true, "short": true, "int": true,
	"long": true, "float": true, "double": true, "void": true,
}

func isJPrimitive(s string) bool { return jPrimitiveSet[s] }

var jModifierSet = map[string]bool{
	"public": true, "private": true, "protected": true, "static": true,
	"final": true, "abstract": true, "transient": true, "volatile": true,
	"synchronized": true, "native": true, "strictfp": true, "sealed": true,
	"non-sealed": true, "default": true,
}

func isJModifier(s string) bool { return jModifierSet[s] }
