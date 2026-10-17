package main

import (
	"strconv"
	"strings"
)

type jParseError struct {
	Message string
	Range   jRange
}

func (e *jParseError) Error() string {
	return strconv.Itoa(e.Range.Start.Line) + ":" + strconv.Itoa(e.Range.Start.Col) + ": " + e.Message
}

type jOpt func(*jParser)

func jWithSink(sink jEventSink) jOpt {
	return func(p *jParser) { p.sink = sink }
}

func jWithPolicy(policy jPolicy) jOpt {
	return func(p *jParser) { p.policy = policy }
}

func jWithSignatureOnly(v bool) jOpt {
	return func(p *jParser) { p.sigOnly = v }
}

func jWithDeltaScope(name string) jOpt {
	return func(p *jParser) { p.delta = name; p.inDelta = true }
}

type jParser struct {
	l         *jLexer
	buf       []jTok
	lastEnd   jPos
	lastTo    int
	sourceID  string
	sink      jEventSink
	policy    jPolicy
	sigOnly   bool
	errs      []*jParseError
	failed    bool
	doc       string
	enumHead  bool
	enumOwner string
	delta     string
	inDelta   bool
}

func newJParser(src, sourceID string, opts ...jOpt) *jParser {
	p := &jParser{
		l:        newJLexer(src),
		sourceID: sourceID,
		policy:   jPolicyDeclarations | jPolicyOccurrences,
	}
	for _, o := range opts {
		o(p)
	}
	if p.policy&jPolicyOccurrences == 0 {
		p.sigOnly = true
	}
	p.fill(1)
	return p
}

func (p *jParser) fill(n int) {
	for len(p.buf) <= n {
		t := p.l.nextTok()
		if t.Type == jTokComment {
			if len(p.buf) == 0 && isJavadoc(t.Literal) {
				p.doc = cleanJavadoc(t.Literal)
			}
			continue
		}
		p.buf = append(p.buf, t)
	}
}

func (p *jParser) cur() jTok {
	p.fill(0)
	return p.buf[0]
}

func (p *jParser) peek() jTok {
	p.fill(1)
	return p.buf[1]
}

func (p *jParser) look(n int) jTok {
	p.fill(n)
	return p.buf[n]
}

func (p *jParser) advance() {
	p.fill(0)
	t := p.buf[0]
	p.lastEnd = t.Range.End
	p.lastTo = t.To
	if t.Type == jTokEOF {
		return
	}
	if t.isSym(";") || t.isSym("}") || t.isSym("{") {
		p.doc = ""
	}
	p.buf = p.buf[1:]
}

func (p *jParser) atEOF() bool { return p.cur().Type == jTokEOF }

func (p *jParser) takeDoc() string {
	d := p.doc
	p.doc = ""
	return d
}

func (p *jParser) emit(evt jEvent) {
	if p.sink == nil || p.failed {
		return
	}
	p.push(evt)
}

func (p *jParser) push(evt jEvent) {
	if p.sink == nil {
		return
	}
	if err := p.sink.Emit(evt); err != nil {
		p.errs = append(p.errs, &jParseError{
			Message: "事件接收器拒绝事件：" + err.Error(), Range: evt.GetRange(),
		})
	}
}

func (p *jParser) errorAt(msg string, r jRange) {
	p.errs = append(p.errs, &jParseError{Message: msg, Range: r})
	if p.failed {
		return
	}
	p.failed = true
	p.push(jParseErrorEvent{SourceID: p.sourceID, Message: msg, Range: r})
}

func (p *jParser) raw(from, to int) string {
	if from < 0 || to > len(p.l.src) || from >= to {
		return ""
	}
	return string(p.l.src[from:to])
}

func (p *jParser) parseFile() (*jFile, error) {
	if p.inDelta {
		block, err := p.parseLocalScope(p.delta)
		if block != nil {
			return &jFile{Path: p.sourceID, Types: nil, Range: block.Range}, err
		}
		return &jFile{Path: p.sourceID}, err
	}
	p.emit(jStartFileEvent{SourceID: p.sourceID})
	file := &jFile{Path: p.sourceID, Range: jRange{Start: p.cur().Range.Start}}
	if p.cur().isKw("package") {
		file.Pkg = p.parsePackage()
	}
	for p.cur().isKw("import") {
		file.Imports = append(file.Imports, p.parseImport())
	}
	for !p.atEOF() {
		td := p.parseTypeDecl()
		if td != nil {
			file.Types = append(file.Types, td)
			continue
		}
		p.syncType()
	}
	file.Range.End = p.lastEnd
	if len(p.errs) > 0 {
		return file, p.errs[0]
	}
	p.emit(jEndFileEvent{SourceID: p.sourceID})
	return file, nil
}

func jParseLocalScope(src, mergeScope string) (*jBlock, error) {
	p := newJParser(src, mergeScope, jWithDeltaScope(mergeScope))
	return p.parseLocalScope(mergeScope)
}

func (p *jParser) parseLocalScope(mergeScope string) (*jBlock, error) {
	start := p.cur().Range.Start
	p.emit(jStartDeltaScopeEvent{SourceID: p.sourceID, MergeScope: mergeScope, Range: jRange{Start: start}})
	block := p.parseBlock()
	if len(p.errs) > 0 {
		return block, p.errs[0]
	}
	p.emit(jEndDeltaScopeEvent{SourceID: p.sourceID, MergeScope: mergeScope, Range: block.Range})
	return block, nil
}

func (p *jParser) parsePackage() *jPkg {
	start := p.cur().Range.Start
	p.advance()
	name := p.parseQualifiedName()
	if p.cur().isSym(";") {
		p.advance()
	} else {
		p.errorAt("package 声明缺少 ';'", p.cur().Range)
	}
	pkg := &jPkg{Name: name, Range: jRange{Start: start, End: p.lastEnd}}
	p.emit(jPackageDeclEvent{SourceID: p.sourceID, Name: name, Range: pkg.Range})
	return pkg
}

func (p *jParser) parseImport() *jImport {
	start := p.cur().Range.Start
	p.advance()
	isStatic := false
	if p.cur().isKw("static") {
		isStatic = true
		p.advance()
	}
	name := p.parseQualifiedName()
	isStar := false
	if p.cur().isSym(".") {
		p.advance()
		switch {
		case p.cur().isSym("*"):
			isStar = true
			p.advance()
		case p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword:
			name += "." + p.cur().Literal
			p.advance()
		default:
			p.errorAt("import 声明缺少名称", p.cur().Range)
		}
	}
	if p.cur().isSym(";") {
		p.advance()
	} else {
		p.errorAt("import 声明缺少 ';'", p.cur().Range)
	}
	imp := &jImport{Name: name, IsStatic: isStatic, IsStar: isStar, Range: jRange{Start: start, End: p.lastEnd}}
	p.emit(jImportDeclEvent{SourceID: p.sourceID, Name: name, IsStatic: isStatic, IsStar: isStar, Range: imp.Range})
	return imp
}

func (p *jParser) parseTypeDecl() *jTypeDecl {
	doc := p.takeDoc()
	annos := p.parseAnnotations()
	mods := p.parseModifiers()
	return p.parseTypeDeclAfter(doc, annos, mods)
}

func (p *jParser) parseTypeDeclAfter(doc string, annos []*jAnno, mods []string) *jTypeDecl {
	var kind jKind
	switch {
	case p.cur().isSym("@") && p.peek().isKw("interface"):
		p.advance()
		p.advance()
		kind = jKindAnnotation
	case p.cur().isKw("class"):
		p.advance()
		kind = jKindClass
	case p.cur().isKw("interface"):
		p.advance()
		kind = jKindInterface
	case p.cur().isKw("enum"):
		p.advance()
		kind = jKindEnum
	case p.cur().isWord("record"):
		p.advance()
		kind = jKindRecord
	default:
		p.errorAt("此处不是类型声明", p.cur().Range)
		return nil
	}
	if p.cur().Type != jTokIdent && p.cur().Type != jTokKeyword {
		p.errorAt("类型声明缺少名称", p.cur().Range)
		return nil
	}
	name := p.cur().Literal
	nameRange := p.cur().Range
	p.advance()
	td := &jTypeDecl{
		Kind:        kind,
		Name:        name,
		Modifiers:   mods,
		Annotations: annos,
		Doc:         doc,
		NameRange:   nameRange,
		Range:       jRange{Start: nameRange.Start},
	}
	var header []*jParam
	if p.cur().isSym("<") {
		td.Generics = p.parseTypeParams()
	}
	if kind == jKindRecord && p.cur().isSym("(") {
		header = p.parseParameters()
	}
	if p.cur().isKw("extends") {
		p.advance()
		td.Extends = p.parseTypeList()
	}
	if p.cur().isKw("implements") {
		p.advance()
		td.Implements = p.parseTypeList()
	}
	if p.cur().isWord("permits") {
		p.advance()
		td.Permits = p.parseTypeList()
	}
	p.emit(jTypeDeclEvent{
		SourceID: p.sourceID, Kind: kind, Name: name, Modifiers: mods,
		Annotations: jAnnoNames(annos), Range: nameRange, Doc: doc,
	})
	for _, hp := range header {
		p.emit(jFieldDeclEvent{
			SourceID: p.sourceID, Name: hp.Name, DataType: hp.Type,
			Modifiers: []string{"private", "final"}, Range: hp.Range,
		})
	}
	if !p.cur().isSym("{") {
		p.errorAt("类型声明缺少 '{'", p.cur().Range)
		td.Range.End = p.lastEnd
		return td
	}
	p.advance()
	p.enumHead = kind == jKindEnum
	p.enumOwner = name
	for !p.cur().isSym("}") {
		if p.atEOF() {
			p.errorAt("类型声明缺少 '}'", p.cur().Range)
			break
		}
		members := p.parseTypeMember(name)
		if len(members) == 0 {
			p.syncMember()
			continue
		}
		td.Members = append(td.Members, members...)
	}
	if p.cur().isSym("}") {
		p.advance()
	}
	td.Range.End = p.lastEnd
	for _, hp := range header {
		hf := &jFieldDecl{
			Name: hp.Name, Type: hp.Type, Modifiers: []string{"private", "final"},
			Range: hp.Range, NameRange: hp.Range,
		}
		td.Members = append([]jMember{hf}, td.Members...)
	}
	return td
}

func (p *jParser) parseTypeMember(className string) []jMember {
	doc := p.takeDoc()
	annos := p.parseAnnotations()
	mods := p.parseModifiers()
	if p.enumHead && (p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword) && p.isEnumConstAhead() {
		return []jMember{p.parseEnumConst(doc, annos, mods)}
	}
	p.enumHead = false
	switch {
	case (p.cur().isSym("@") && p.peek().isKw("interface")) ||
		p.cur().isKw("class") || p.cur().isKw("interface") ||
		p.cur().isKw("enum") || p.cur().isWord("record"):
		td := p.parseTypeDeclAfter(doc, annos, mods)
		if td == nil {
			return nil
		}
		return []jMember{td}
	}
	if p.cur().isSym("{") {
		body := p.parseBlock()
		ib := &jInitBlock{Static: hasWord(mods, "static"), Body: body, Range: body.Range, Doc: doc}
		p.emit(jInitBlockEvent{SourceID: p.sourceID, Static: ib.Static, Range: body.Range})
		return []jMember{ib}
	}
	if (p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword) && p.cur().Literal == className &&
		(p.peek().isSym("(") || p.peek().isSym("{")) {
		return []jMember{p.parseCtor(doc, mods, annos)}
	}
	var generics []*jTypeParam
	if p.cur().isSym("<") {
		generics = p.parseTypeParams()
	}
	if !p.isTypeStart() {
		p.errorAt("成员声明缺少类型", p.cur().Range)
		return nil
	}
	typ := p.skipTypeTokens()
	if p.cur().Type != jTokIdent && p.cur().Type != jTokKeyword {
		p.errorAt("成员声明缺少名称", p.cur().Range)
		return nil
	}
	name := p.cur().Literal
	nameRange := p.cur().Range
	p.advance()
	if p.cur().isSym("(") {
		m := p.parseMethod(doc, mods, annos, generics, typ, name, nameRange)
		if m == nil {
			return nil
		}
		return []jMember{m}
	}
	return p.parseFieldDecls(doc, mods, annos, typ, name, nameRange)
}

func (p *jParser) isEnumConstAhead() bool {
	n := p.peek()
	return n.isSym(",") || n.isSym(";") || n.isSym("(") || n.isSym("{") || n.isSym("}")
}

func (p *jParser) parseEnumConst(doc string, annos []*jAnno, mods []string) *jFieldDecl {
	name := p.cur().Literal
	nr := p.cur().Range
	p.advance()
	if p.cur().isSym("(") {
		p.skipParens()
	}
	if p.cur().isSym("{") {
		p.skipBraces()
	}
	fd := &jFieldDecl{
		Name: name, Type: p.enumOwner, Modifiers: mods, Annotations: annos,
		IsEnumConst: true, Range: nr, NameRange: nr, Doc: doc,
	}
	p.emit(jFieldDeclEvent{
		SourceID: p.sourceID, Name: name, DataType: p.enumOwner, Modifiers: mods,
		Annotations: jAnnoNames(annos), IsEnumConst: true, Range: nr, Doc: doc,
	})
	if p.cur().isSym(",") {
		p.advance()
	} else if p.cur().isSym(";") {
		p.advance()
		p.enumHead = false
	}
	return fd
}

func (p *jParser) parseFieldDecls(doc string, mods []string, annos []*jAnno, typ, name string, nameRange jRange) []jMember {
	var out []jMember
	hadInit := false
	for {
		fd := &jFieldDecl{
			Name: name, Type: typ, Modifiers: mods, Annotations: annos,
			Range: nameRange, NameRange: nameRange, Doc: doc,
		}
		p.emit(jFieldDeclEvent{
			SourceID: p.sourceID, Name: name, DataType: typ, Modifiers: mods,
			Annotations: jAnnoNames(annos), Range: nameRange, Doc: doc,
		})
		out = append(out, fd)
		if p.cur().isSym("=") {
			p.advance()
			p.skipToSemicolon()
			hadInit = true
		}
		if !p.cur().isSym(",") {
			break
		}
		p.advance()
		if p.cur().Type != jTokIdent && p.cur().Type != jTokKeyword {
			p.errorAt("字段声明缺少名称", p.cur().Range)
			return out
		}
		name = p.cur().Literal
		nameRange = p.cur().Range
		p.advance()
	}
	if p.cur().isSym(";") {
		p.advance()
	} else if !hadInit {
		p.errorAt("字段声明缺少 ';'", p.cur().Range)
	}
	return out
}

func (p *jParser) parseMethod(doc string, mods []string, annos []*jAnno, generics []*jTypeParam, ret, name string, nameRange jRange) *jMethodDecl {
	params := p.parseParameters()
	var exceptions []string
	if p.cur().isKw("throws") {
		p.advance()
		exceptions = p.parseTypeList()
	}
	m := &jMethodDecl{
		Name: name, ReturnType: ret, Modifiers: mods, Annotations: annos,
		Generics: generics, Parameters: params, Exceptions: exceptions,
		NameRange: nameRange, Doc: doc, Range: jRange{Start: nameRange.Start},
	}
	p.emit(jMethodDeclEvent{
		SourceID: p.sourceID, Name: name, ReturnType: ret, Modifiers: mods,
		Annotations: jAnnoNames(annos), Parameters: params, Exceptions: exceptions,
		Range: nameRange, Doc: doc,
	})
	switch {
	case p.cur().isSym(";"):
		p.advance()
	case p.cur().isKw("default"):
		p.advance()
		from := p.cur().From
		m.Default = strings.TrimSpace(p.raw(from, p.skipToSemicolon()))
	case p.cur().isSym("{"):
		if p.sigOnly {
			p.skipBraces()
		} else {
			m.Body = p.parseBlock()
		}
	default:
		p.errorAt("方法声明缺少 '{' 或 ';'", p.cur().Range)
	}
	m.Range.End = p.lastEnd
	return m
}

func (p *jParser) parseCtor(doc string, mods []string, annos []*jAnno) *jCtorDecl {
	name := p.cur().Literal
	nr := p.cur().Range
	p.advance()
	params := p.parseParameters()
	var exceptions []string
	if p.cur().isKw("throws") {
		p.advance()
		exceptions = p.parseTypeList()
	}
	c := &jCtorDecl{
		Name: name, Modifiers: mods, Annotations: annos, Parameters: params,
		Exceptions: exceptions, NameRange: nr, Doc: doc, Range: jRange{Start: nr.Start},
	}
	p.emit(jCtorDeclEvent{
		SourceID: p.sourceID, Name: name, Modifiers: mods,
		Annotations: jAnnoNames(annos), Parameters: params, Exceptions: exceptions,
		Range: nr, Doc: doc,
	})
	if p.cur().isSym("{") {
		if p.sigOnly {
			p.skipBraces()
		} else {
			c.Body = p.parseBlock()
		}
	} else {
		p.errorAt("构造器声明缺少 '{'", p.cur().Range)
	}
	c.Range.End = p.lastEnd
	return c
}

func (p *jParser) parseBlock() *jBlock {
	start := p.cur().Range.Start
	p.advance()
	b := &jBlock{Range: jRange{Start: start}}
	p.emit(jScopeEnterEvent{Range: b.Range})
	for !p.cur().isSym("}") {
		if p.atEOF() {
			p.errorAt("代码块缺少 '}'", p.cur().Range)
			break
		}
		if p.cur().isSym("{") {
			b.Locals = append(b.Locals, p.parseBlock())
			continue
		}
		if p.maybeLocalDecl() {
			if _, ok := p.localDeclAhead(); ok {
				if lv := p.parseLocalVar(); lv != nil {
					b.Locals = append(b.Locals, lv)
					continue
				}
			}
		}
		p.advance()
	}
	if p.cur().isSym("}") {
		p.advance()
	}
	b.Range.End = p.lastEnd
	p.emit(jScopeLeaveEvent{Range: b.Range})
	return b
}

func (p *jParser) maybeLocalDecl() bool {
	c := p.cur()
	if c.isSym("final") || c.isSym("@") {
		return true
	}
	if !p.isTypeTok(c) {
		return false
	}
	n := p.peek()
	return n.Type == jTokIdent || n.Type == jTokKeyword || n.isSym("<") || n.isSym("[") || n.isSym(".")
}

func (p *jParser) localDeclAhead() (int, bool) {
	i := 0
	if p.look(i).isSym("final") {
		i++
	}
	for p.look(i).isSym("@") {
		i++
		for p.look(i).Type == jTokIdent || p.look(i).Type == jTokKeyword {
			i++
		}
		if p.look(i).isSym("(") {
			depth := 0
			for {
				t := p.look(i)
				if t.Type == jTokEOF {
					return 0, false
				}
				if t.isSym("(") {
					depth++
				} else if t.isSym(")") {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				i++
			}
		}
	}
	if !p.isTypeTok(p.look(i)) {
		return 0, false
	}
	i++
	for p.look(i).isSym(".") && (p.look(i+1).Type == jTokIdent || p.look(i+1).Type == jTokKeyword) {
		i += 2
	}
	if p.look(i).isSym("<") {
		depth := 0
		for guard := 0; guard < 96; guard++ {
			t := p.look(i)
			if t.Type == jTokEOF {
				return 0, false
			}
			if t.isSym("<") {
				depth++
			} else if t.isSym(">") {
				depth--
				if depth == 0 {
					i++
					break
				}
			}
			i++
		}
	}
	for p.look(i).isSym("[") && p.look(i+1).isSym("]") {
		i += 2
	}
	if p.look(i).isSym("...") {
		i++
	}
	if p.look(i).Type != jTokIdent && p.look(i).Type != jTokKeyword {
		return 0, false
	}
	i++
	a := p.look(i)
	if !(a.isSym("=") || a.isSym(";") || a.isSym(",") || a.isSym(":") || a.isSym(")")) {
		return 0, false
	}
	return i, true
}

func (p *jParser) parseLocalVar() *jLocalVar {
	if p.cur().isSym("final") {
		p.advance()
	}
	p.parseAnnotations()
	mods := p.parseModifiers()
	if p.cur().isSym("final") {
		mods = append(mods, "final")
		p.advance()
	}
	typ := p.skipTypeTokens()
	if p.cur().Type != jTokIdent && p.cur().Type != jTokKeyword {
		return nil
	}
	name := p.cur().Literal
	nr := p.cur().Range
	p.advance()
	for p.cur().isSym("[") && p.peek().isSym("]") {
		p.advance()
		p.advance()
	}
	lv := &jLocalVar{Name: name, Type: typ, Modifiers: mods, Range: nr, NameRange: nr}
	p.emit(jLocalDeclEvent{SourceID: p.sourceID, Name: name, DataType: typ, Range: nr})
	if p.cur().isSym("=") {
		p.advance()
		p.skipToSemicolon()
	}
	for p.cur().isSym(",") {
		p.advance()
		if p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword {
			p.advance()
			p.skipToSemicolon()
			continue
		}
		break
	}
	return lv
}

func (p *jParser) parseParameters() []*jParam {
	if !p.cur().isSym("(") {
		return nil
	}
	p.advance()
	var out []*jParam
	for !p.cur().isSym(")") {
		if p.atEOF() {
			p.errorAt("参数列表缺少 ')'", p.cur().Range)
			return out
		}
		if p.cur().isSym("{") || p.cur().isSym(";") {
			p.errorAt("参数列表缺少 ')'", p.cur().Range)
			return out
		}
		annos := p.parseAnnotations()
		mods := p.parseModifiers()
		typ := p.skipTypeTokens()
		varArgs := false
		if p.cur().isSym("...") {
			varArgs = true
			p.advance()
		}
		var name string
		var nr jRange
		if p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword {
			name = p.cur().Literal
			nr = p.cur().Range
			p.advance()
		}
		for p.cur().isSym("[") && p.peek().isSym("]") {
			p.advance()
			p.advance()
		}
		out = append(out, &jParam{
			Name: name, Type: typ, Annotations: jAnnoNames(annos),
			Modifiers: mods, VarArgs: varArgs, Range: nr,
		})
		if p.cur().isSym(",") {
			p.advance()
		} else if !p.cur().isSym(")") {
			p.advance()
		}
	}
	p.advance()
	return out
}

func (p *jParser) parseTypeParams() []*jTypeParam {
	if !p.cur().isSym("<") {
		return nil
	}
	p.advance()
	var out []*jTypeParam
	for !p.cur().isSym(">") {
		if p.atEOF() {
			p.errorAt("泛型参数列表缺少 '>'", p.cur().Range)
			return out
		}
		if p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword {
			name := p.cur().Literal
			r := p.cur().Range
			p.advance()
			var bounds []string
			if p.cur().isKw("extends") {
				p.advance()
				bounds = append(bounds, p.skipTypeTokens())
				for p.cur().isSym("&") {
					p.advance()
					bounds = append(bounds, p.skipTypeTokens())
				}
			}
			out = append(out, &jTypeParam{Name: name, Bounds: bounds, Range: jRange{Start: r.Start, End: p.lastEnd}})
		} else {
			p.advance()
		}
		if p.cur().isSym(",") {
			p.advance()
		}
	}
	p.advance()
	return out
}

func (p *jParser) parseTypeList() []string {
	var out []string
	out = append(out, p.skipTypeTokens())
	for p.cur().isSym(",") {
		p.advance()
		out = append(out, p.skipTypeTokens())
	}
	return out
}

func (p *jParser) parseQualifiedName() string {
	var b strings.Builder
	if p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword {
		b.WriteString(p.cur().Literal)
		p.advance()
	}
	for p.cur().isSym(".") && (p.peek().Type == jTokIdent || p.peek().Type == jTokKeyword) {
		b.WriteByte('.')
		p.advance()
		b.WriteString(p.cur().Literal)
		p.advance()
	}
	return b.String()
}

func (p *jParser) parseAnnotations() []*jAnno {
	var out []*jAnno
	for p.cur().isSym("@") && !p.peek().isKw("interface") {
		start := p.cur().Range.Start
		p.advance()
		if p.cur().Type != jTokIdent && p.cur().Type != jTokKeyword {
			p.errorAt("注解缺少名称", p.cur().Range)
			break
		}
		name := p.parseQualifiedName()
		if p.cur().isSym("(") {
			p.skipParens()
		}
		out = append(out, &jAnno{Name: name, Range: jRange{Start: start, End: p.lastEnd}})
	}
	return out
}

func (p *jParser) parseModifiers() []string {
	var out []string
	for {
		t := p.cur()
		if t.Type == jTokKeyword && isJModifier(t.Literal) {
			out = append(out, t.Literal)
			p.advance()
			continue
		}
		if t.Type == jTokIdent && (t.Literal == "sealed" || t.Literal == "non-sealed") {
			out = append(out, t.Literal)
			p.advance()
			continue
		}
		break
	}
	return out
}

func (p *jParser) skipTypeTokens() string {
	from := p.cur().From
	if p.cur().Type == jTokIdent || p.cur().Type == jTokKeyword {
		p.advance()
	}
	for p.cur().isSym(".") && (p.peek().Type == jTokIdent || p.peek().Type == jTokKeyword) {
		p.advance()
		p.advance()
	}
	if p.cur().isSym("<") {
		p.skipAngle()
	}
	for p.cur().isSym("[") && p.peek().isSym("]") {
		p.advance()
		p.advance()
	}
	return strings.TrimSpace(p.raw(from, p.lastTo))
}

func (p *jParser) skipAngle() string {
	from := p.cur().From
	depth := 0
	for {
		t := p.cur()
		if t.Type == jTokEOF {
			return strings.TrimSpace(p.raw(from, p.lastTo))
		}
		if t.isSym("<") {
			depth++
		} else if t.isSym(">") {
			depth--
		}
		p.advance()
		if depth == 0 {
			break
		}
	}
	return strings.TrimSpace(p.raw(from, p.lastTo))
}

func (p *jParser) skipParens() {
	if !p.cur().isSym("(") {
		return
	}
	depth := 0
	for {
		t := p.cur()
		if t.Type == jTokEOF {
			return
		}
		if t.isSym("(") {
			depth++
		} else if t.isSym(")") {
			depth--
		}
		p.advance()
		if depth == 0 {
			return
		}
	}
}

func (p *jParser) skipBraces() {
	if !p.cur().isSym("{") {
		return
	}
	depth := 0
	for {
		t := p.cur()
		if t.Type == jTokEOF {
			return
		}
		if t.isSym("{") {
			depth++
		} else if t.isSym("}") {
			depth--
		}
		p.advance()
		if depth == 0 {
			return
		}
	}
}

func (p *jParser) skipToSemicolon() int {
	depth := 0
	for {
		t := p.cur()
		if t.Type == jTokEOF {
			return len(p.l.src)
		}
		switch {
		case t.isSym("{") || t.isSym("(") || t.isSym("["):
			depth++
		case t.isSym("}") || t.isSym(")") || t.isSym("]"):
			if depth == 0 {
				return t.From
			}
			depth--
		case t.isSym(";"):
			if depth == 0 {
				end := t.From
				p.advance()
				return end
			}
		}
		p.advance()
	}
}

func (p *jParser) syncMember() {
	if p.atEOF() {
		return
	}
	first := true
	for !p.atEOF() {
		if !first && p.atMemberBoundary() {
			return
		}
		p.advance()
		first = false
	}
}

func (p *jParser) atMemberBoundary() bool {
	t := p.cur()
	if t.isSym(";") || t.isSym("{") || t.isSym("}") || t.isSym("@") {
		return true
	}
	if t.Type == jTokKeyword {
		return isJModifier(t.Literal) || isJPrimitive(t.Literal) ||
			t.Literal == "class" || t.Literal == "interface" || t.Literal == "enum"
	}
	return t.Type == jTokIdent
}

func (p *jParser) syncType() {
	if p.atEOF() {
		return
	}
	first := true
	for !p.atEOF() {
		if !first && (p.cur().isKw("class") || p.cur().isKw("interface") ||
			p.cur().isKw("enum") || p.cur().isWord("record") ||
			(p.cur().isSym("@") && p.peek().isKw("interface"))) {
			return
		}
		p.advance()
		first = false
	}
}

func (p *jParser) isTypeStart() bool {
	t := p.cur()
	return t.Type == jTokIdent || (t.Type == jTokKeyword && (isJPrimitive(t.Literal) || t.Literal == "var"))
}

func (p *jParser) isTypeTok(t jTok) bool {
	return t.Type == jTokIdent || (t.Type == jTokKeyword && (isJPrimitive(t.Literal) || t.Literal == "var"))
}

func hasWord(list []string, w string) bool {
	for _, s := range list {
		if s == w {
			return true
		}
	}
	return false
}

func isJavadoc(lit string) bool {
	return strings.HasPrefix(lit, "/**") && len(lit) > 4
}

func cleanJavadoc(lit string) string {
	if !isJavadoc(lit) {
		return ""
	}
	body := strings.TrimSuffix(strings.TrimPrefix(lit, "/**"), "*/")
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
