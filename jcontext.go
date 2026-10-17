package main

import (
	"strings"
)

func (p jPos) less(q jPos) bool { return p.Line < q.Line || (p.Line == q.Line && p.Col < q.Col) }

type jRecvKind int

const (
	jRecvNone jRecvKind = iota
	jRecvSimpleName
	jRecvQualified
	jRecvMethodCall
	jRecvNew
)

type jReceiver struct {
	Raw   string
	Kind  jRecvKind
	Range jRange
}

type jCursor struct {
	File     string
	Pos      jPos
	Package  string
	Type     string
	Method   string
	Imports  []string
	Locals   []*jLocalVar
	Params   []*jParam
	Members  []jMember
	Prefix   string
	Receiver *jReceiver
}

func jExtractCursor(file, src string, line, col int) *jCursor {
	cur := &jCursor{File: file, Pos: jPos{Line: line, Col: col}}
	p := newJParser(src, file)
	f, _ := p.parseFile()
	if f == nil {
		jFillPrefix(cur, src)
		return cur
	}
	if f.Pkg != nil {
		cur.Package = f.Pkg.Name
	}
	for _, im := range f.Imports {
		cur.Imports = append(cur.Imports, im.Name)
	}
	at := cur.Pos
	for _, td := range f.Types {
		if !td.Range.contains(at) {
			continue
		}
		jFillScope(cur, jInnermostType(td, at), at)
		break
	}
	jFillPrefix(cur, src)
	return cur
}

func jInnermostType(td *jTypeDecl, at jPos) *jTypeDecl {
	best := td
	for _, m := range td.Members {
		nt, ok := m.(*jTypeDecl)
		if !ok || !nt.Range.contains(at) {
			continue
		}
		best = jInnermostType(nt, at)
	}
	return best
}

func jFillScope(cur *jCursor, td *jTypeDecl, at jPos) {
	cur.Type = td.Name
	cur.Members = td.Members
	for _, m := range td.Members {
		switch d := m.(type) {
		case *jMethodDecl:
			if d.Range.contains(at) {
				cur.Method = d.Name
				cur.Params = d.Parameters
				jCollectLocals(d.Body, at, &cur.Locals)
			}
		case *jCtorDecl:
			if d.Range.contains(at) {
				cur.Method = d.Name
				cur.Params = d.Parameters
				jCollectLocals(d.Body, at, &cur.Locals)
			}
		case *jInitBlock:
			if d.Range.contains(at) {
				jCollectLocals(d.Body, at, &cur.Locals)
			}
		}
	}
}

func jCollectLocals(b *jBlock, at jPos, out *[]*jLocalVar) {
	if b == nil {
		return
	}
	for _, n := range b.Locals {
		switch v := n.(type) {
		case *jLocalVar:
			if v.NameRange.Start.less(at) || v.NameRange.Start == at {
				*out = append(*out, v)
			}
		case *jBlock:
			if v.Range.contains(at) {
				jCollectLocals(v, at, out)
			}
		}
	}
}

func jFillPrefix(cur *jCursor, src string) {
	text := strings.TrimSuffix(jLineAt(src, cur.Pos.Line), "\r")
	col := cur.Pos.Col
	if col < 0 {
		col = 0
	}
	if col > len(text) {
		col = len(text)
	}
	left := text[:col]
	i := len(left)
	for i > 0 && isJIdentByte(left[i-1]) {
		i--
	}
	cur.Prefix = left[i:]
	if i == 0 || left[i-1] != '.' {
		return
	}
	dot := i - 1
	kind := jRecvSimpleName
	end := dot
	if dot > 0 && left[dot-1] == ')' {
		if open := jMatchParenBack(left, dot-1); open >= 0 {
			end = open
			kind = jRecvMethodCall
		}
	}
	start := end
	for {
		if start > 0 && isJIdentByte(left[start-1]) {
			for start > 0 && isJIdentByte(left[start-1]) {
				start--
			}
			continue
		}
		if start > 0 && left[start-1] == '.' {
			k := start - 1
			for k > 0 && isJIdentByte(left[k-1]) {
				k--
			}
			if k >= start-1 {
				break
			}
			start = k
			if kind == jRecvSimpleName {
				kind = jRecvQualified
			}
			continue
		}
		break
	}
	if start >= end {
		return
	}
	raw := left[start:end]
	if kind == jRecvSimpleName && jHasWordBefore(left, start, "new") {
		kind = jRecvNew
	}
	cur.Receiver = &jReceiver{
		Raw: raw, Kind: kind,
		Range: jRange{Start: jPos{Line: cur.Pos.Line, Col: start}, End: jPos{Line: cur.Pos.Line, Col: end}},
	}
}

func jLineAt(src string, line int) string {
	if line < 1 {
		return ""
	}
	rest := src
	for n := 1; n < line; n++ {
		i := strings.IndexByte(rest, '\n')
		if i < 0 {
			return ""
		}
		rest = rest[i+1:]
	}
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		return rest[:i]
	}
	return rest
}

func jMatchParenBack(s string, close int) int {
	depth := 0
	for j := close; j >= 0; j-- {
		switch s[j] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func jHasWordBefore(text string, end int, word string) bool {
	if end < len(word)+1 {
		return false
	}
	if !strings.HasSuffix(text[:end], " "+word) {
		return false
	}
	i := end - len(word) - 1
	return i == 0 || !isJIdentByte(text[i-1])
}
