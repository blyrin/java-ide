package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type javaKind int

const (
	javaPackage javaKind = iota
	javaImport
	javaClass
	javaInterface
	javaEnum
	javaRecord
	javaAnnotationType
	javaMethod
	javaConstructor
	javaField
	javaEnumConst
)

func (k javaKind) label() string {
	switch k {
	case javaPackage:
		return "package"
	case javaImport:
		return "import"
	case javaInterface:
		return "interface"
	case javaEnum:
		return "enum"
	case javaRecord:
		return "record"
	case javaAnnotationType:
		return "annotation"
	case javaMethod:
		return "method"
	case javaConstructor:
		return "constructor"
	case javaEnumConst:
		return "constant"
	default:
		return "field"
	}
}

func jKindToJava(k jKind) javaKind {
	switch k {
	case jKindInterface:
		return javaInterface
	case jKindEnum:
		return javaEnum
	case jKindRecord:
		return javaRecord
	case jKindAnnotation:
		return javaAnnotationType
	default:
		return javaClass
	}
}

type javaMember struct {
	kind     javaKind
	name     string
	detail   string
	doc      string
	line     int
	col      int
	depth    int
	children []*javaMember
}

func jParamsText(params []*jParam) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, p.text())
	}
	return strings.Join(parts, ", ")
}

func jMemberDetail(kind javaKind, typ, name string, params []*jParam) string {
	switch kind {
	case javaMethod:
		return typ + " " + name + "(" + jParamsText(params) + ")"
	case javaConstructor:
		return name + "(" + jParamsText(params) + ")"
	case javaEnumConst:
		return name
	case javaPackage, javaImport:
		return name
	default:
		if typ == "" {
			return name
		}
		return typ + " " + name
	}
}

func parseJavaStructure(src string) []*javaMember {
	p := newJParser(src, "", jWithSignatureOnly(true))
	f, _ := p.parseFile()
	if f == nil {
		return nil
	}
	var roots []*javaMember
	for _, td := range f.Types {
		if m := jTypeMember(td); m != nil {
			roots = append(roots, m)
		}
	}
	return roots
}

func jTypeMember(td *jTypeDecl) *javaMember {
	kind := jKindToJava(td.Kind)
	m := &javaMember{
		kind:   kind,
		name:   td.Name,
		detail: jMemberDetail(kind, td.Kind.String(), td.Name, nil),
		doc:    td.Doc,
		line:   td.NameRange.Start.Line - 1,
		col:    td.NameRange.Start.Col - 1,
	}
	for _, mem := range td.Members {
		if c := jDeclMember(mem); c != nil {
			m.children = append(m.children, c)
		}
	}
	return m
}

func jDeclMember(mem jMember) *javaMember {
	switch d := mem.(type) {
	case *jTypeDecl:
		return jTypeMember(d)
	case *jFieldDecl:
		kind := javaField
		if d.IsEnumConst {
			kind = javaEnumConst
		}
		return &javaMember{
			kind: kind, name: d.Name, detail: jMemberDetail(kind, d.Type, d.Name, nil),
			doc: d.Doc, line: d.NameRange.Start.Line - 1, col: d.NameRange.Start.Col - 1,
		}
	case *jMethodDecl:
		return &javaMember{
			kind: javaMethod, name: d.Name,
			detail: jMemberDetail(javaMethod, d.ReturnType, d.Name, d.Parameters),
			doc:    d.Doc, line: d.NameRange.Start.Line - 1, col: d.NameRange.Start.Col - 1,
		}
	case *jCtorDecl:
		return &javaMember{
			kind: javaConstructor, name: d.Name,
			detail: jMemberDetail(javaConstructor, "", d.Name, d.Parameters),
			doc:    d.Doc, line: d.NameRange.Start.Line - 1, col: d.NameRange.Start.Col - 1,
		}
	}
	return nil
}

type jSymbol struct {
	name   string
	kind   javaKind
	detail string
	doc    string
	file   string
	line   int
	col    int
}

type symbolIndex struct {
	mu      sync.RWMutex
	symbols map[string][]jSymbol
}

func (ix *symbolIndex) ready() bool {
	return ix != nil && ix.symbols != nil
}

func (ix *symbolIndex) add(sym jSymbol) {
	if ix == nil || ix.symbols == nil {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.symbols[sym.name] = append(ix.symbols[sym.name], sym)
}

func (ix *symbolIndex) lookup(name string) []jSymbol {
	if !ix.ready() {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.symbols[name]
}

func (ix *symbolIndex) match(prefix string, limit int) []jSymbol {
	if !ix.ready() || prefix == "" {
		return nil
	}
	ix.mu.RLock()
	var out []jSymbol
	for name, list := range ix.symbols {
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		out = append(out, list...)
	}
	ix.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].file < out[j].file
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func jSymbolFromFact(file string, kind javaKind, name, detail, doc string, r jRange) jSymbol {
	return jSymbol{
		name: name, kind: kind, detail: detail, doc: doc, file: file,
		line: r.Start.Line, col: r.Start.Col,
	}
}

func buildSymbolIndex(root string) (*symbolIndex, []buildError) {
	ix := &symbolIndex{symbols: map[string][]jSymbol{}}
	var todos []buildError
	for _, file := range javaSourceFiles(root) {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		src := string(b)
		st := newJStaging(file)
		p := newJParser(src, file, jWithSink(st), jWithPolicy(jPolicyDeclarations))
		p.parseFile()
		facts := st.commit()
		if facts == nil {
			continue
		}
		for _, t := range facts.Types {
			kind := jKindToJava(t.Kind)
			ix.add(jSymbolFromFact(file, kind, t.Name,
				jMemberDetail(kind, t.Kind.String(), t.Name, nil), t.Doc, t.Range))
		}
		for _, m := range facts.Methods {
			kind := javaMethod
			if m.IsCtor {
				kind = javaConstructor
			}
			ix.add(jSymbolFromFact(file, kind, m.Name,
				jMemberDetail(kind, m.ReturnType, m.Name, m.Parameters), m.Doc, m.Range))
		}
		for _, f := range facts.Fields {
			kind := javaField
			if f.IsEnumConst {
				kind = javaEnumConst
			}
			ix.add(jSymbolFromFact(file, kind, f.Name,
				jMemberDetail(kind, f.DataType, f.Name, nil), f.Doc, f.Range))
		}
		todos = append(todos, todosIn(src, file)...)
	}
	return ix, todos
}

var javaKeywords = []string{
	"abstract", "assert", "boolean", "break", "byte", "case", "catch", "char",
	"class", "const", "continue", "default", "do", "double", "else", "enum",
	"extends", "final", "finally", "float", "for", "goto", "if", "implements",
	"import", "instanceof", "int", "interface", "long", "native", "new",
	"package", "private", "protected", "public", "return", "short", "static",
	"strictfp", "super", "switch", "synchronized", "this", "throw", "throws",
	"transient", "try", "void", "volatile", "while", "var", "record", "sealed",
	"permits", "true", "false", "null",
}

type javaSnippet struct {
	label  string
	insert string
	detail string
}

var javaSnippets = []javaSnippet{
	{"sout", "System.out.println();", "打印到标准输出"},
	{"soutf", "System.out.printf();", "格式化输出"},
	{"serr", "System.err.println();", "打印到标准错误"},
	{"psvm", "public static void main(String[] args) {\n    \n}", "main 方法"},
	{"main", "public static void main(String[] args) {\n    \n}", "main 方法"},
	{"fori", "for (int i = 0; i < ; i++) {\n    \n}", "计数循环"},
	{"foreach", "for ( : ) {\n    \n}", "遍历循环"},
	{"iferr", "if (err != null) {\n    \n}", "错误判断"},
	{"toStr", "@Override\npublic String toString() {\n    return ;\n}", "toString 方法"},
}

func jMatches(name, lower string) bool {
	return strings.HasPrefix(strings.ToLower(name), lower)
}

func (s *session) completionsFor(req editor.CompletionRequest) []editor.Completion {
	prefix := req.Prefix
	var out []editor.Completion
	if prefix == "" {
		return out
	}
	lower := strings.ToLower(prefix)
	for _, kw := range javaKeywords {
		if strings.HasPrefix(kw, lower) {
			out = append(out, editor.Completion{Label: kw, Kind: editor.CompletionKeyword})
		}
	}
	for _, sn := range javaSnippets {
		if strings.HasPrefix(sn.label, lower) {
			out = append(out, editor.Completion{
				Label: sn.label, Detail: sn.detail, Kind: editor.CompletionSnippet,
				Insert: sn.insert, Documentation: "代码模板：" + sn.detail,
			})
		}
	}
	for _, c := range s.scopeCompletions(req, lower) {
		out = append(out, c)
	}
	if ix := s.symbols; ix.ready() {
		for _, sym := range ix.match(prefix, 40) {
			kind := editor.CompletionText
			switch sym.kind {
			case javaClass, javaInterface, javaEnum, javaRecord, javaAnnotationType:
				kind = editor.CompletionType
			case javaMethod, javaConstructor:
				kind = editor.CompletionMethod
			case javaField, javaEnumConst:
				kind = editor.CompletionField
			}
			c := editor.Completion{Label: sym.name, Detail: sym.detail, Kind: kind, Documentation: sym.doc}
			out = append(out, c)
		}
	}
	return out
}

func (s *session) scopeCompletions(req editor.CompletionRequest, lower string) []editor.Completion {
	if s.ed == nil || s.ed.Buffer == nil || req.Offset <= 0 {
		return nil
	}
	pos := s.ed.Buffer.Pos(req.Offset)
	cur := jExtractCursor(s.currentPath, s.editorText(), pos.Line, pos.Col)
	var out []editor.Completion
	for _, lv := range cur.Locals {
		if !jMatches(lv.Name, lower) {
			continue
		}
		out = append(out, editor.Completion{
			Label: lv.Name, Detail: lv.Type, Kind: editor.CompletionVariable,
			Documentation: "局部变量",
		})
	}
	for _, p := range cur.Params {
		if !jMatches(p.Name, lower) {
			continue
		}
		out = append(out, editor.Completion{
			Label: p.Name, Detail: p.text(), Kind: editor.CompletionVariable,
			Documentation: "方法参数",
		})
	}
	for _, m := range cur.Members {
		mem := jDeclMember(m)
		if mem == nil || !jMatches(mem.name, lower) {
			continue
		}
		kind := editor.CompletionText
		switch mem.kind {
		case javaMethod, javaConstructor:
			kind = editor.CompletionMethod
		case javaField, javaEnumConst:
			kind = editor.CompletionField
		default:
			kind = editor.CompletionType
		}
		out = append(out, editor.Completion{
			Label: mem.name, Detail: mem.detail, Kind: kind, Documentation: mem.doc,
		})
	}
	return out
}

func (s *session) onCompletion(req editor.CompletionRequest) {
	start := req.Offset - len(req.Prefix)
	s.ed.ShowCompletions(start, s.completionsFor(req))
}

func wordAt(text string, offset int) (word string, start, end int) {
	if offset > len(text) {
		offset = len(text)
	}
	isWord := func(c byte) bool {
		return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	start = offset
	for start > 0 && isWord(text[start-1]) {
		start--
	}
	end = offset
	for end < len(text) && isWord(text[end]) {
		end++
	}
	return text[start:end], start, end
}

func (s *session) onHover(req editor.HoverRequest) {
	if s.ed == nil || s.ed.Buffer == nil {
		return
	}
	word, start, end := wordAt(s.editorText(), req.Offset)
	if word == "" {
		return
	}
	syms := s.symbols.lookup(word)
	if len(syms) == 0 {
		return
	}
	sym := syms[0]
	file, line := sym.file, sym.line
	s.ed.ShowHover(start, end, sym.detail, func(c *ui.Context) {
		t := c.Theme()
		ui.Column(c).Gap(t.Space(1)).MaxWidth(t.Rem(24)).Children(func() {
			if sym.doc != "" {
				ui.Text(c, sym.doc).FontSize(t.Rem(0.85)).TextColor(t.TextMuted)
			}
			ui.Text(c, filepath.ToSlash(file)+":"+strconv.Itoa(line)).FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
	})
}

func (s *session) onDefinition(offset int) {
	if s.ed == nil || s.ed.Buffer == nil {
		return
	}
	word, _, _ := wordAt(s.editorText(), offset)
	if word == "" {
		return
	}
	syms := s.symbols.lookup(word)
	if len(syms) == 0 {
		return
	}
	sym := syms[0]
	if filepath.Clean(sym.file) == filepath.Clean(s.currentPath) {
		s.ed.GoTo(editor.Pos{Line: sym.line - 1, Col: max(sym.col-1, 0)})
		return
	}
	s.app.openFileAt(sym.file, sym.line, sym.col)
}

func (s *session) indexSymbols() {
	go func() {
		ix, todos := buildSymbolIndex(s.path)
		s.symbols = ix
		s.todos = todos
	}()
}

var compileErrorRe = regexp.MustCompile(`^(.+\.java):(\d+)(?::(\d+))?:\s*(.+)$`)

func parseErrorLocation(line string) *sourceLoc {
	m := compileErrorRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return nil
	}
	n, _ := strconv.Atoi(m[2])
	col := 0
	if m[3] != "" {
		col, _ = strconv.Atoi(m[3])
	}
	return &sourceLoc{file: m[1], line: n, col: col}
}

func (s *session) collectBuildErrors(output string) []buildError {
	var out []buildError
	for _, line := range strings.Split(output, "\n") {
		m := compileErrorRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		msg := strings.TrimSpace(m[4])
		out = append(out, buildError{
			file: parseSourcePath(s.path, m[1]), line: n, col: col,
			msg: msg, warn: strings.Contains(msg, "warning"),
		})
	}
	return out
}

func parseSourcePath(root, file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(root, file)
}
