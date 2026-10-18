package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type exprKind int

const (
	ekVoid exprKind = iota
	ekBoolean
	ekByte
	ekChar
	ekShort
	ekInt
	ekLong
	ekFloat
	ekDouble
	ekString
	ekObject
	ekArray
	ekNull
)

type exprVal struct {
	kind  exprKind
	i     int64
	f     float64
	s     string
	obj   int64
	name  string
	ident string
}

func (v exprVal) numeric() bool {
	switch v.kind {
	case ekByte, ekChar, ekShort, ekInt, ekLong, ekFloat, ekDouble:
		return true
	}
	return false
}

func (v exprVal) asFloat() float64 {
	if v.kind == ekFloat || v.kind == ekDouble {
		return v.f
	}
	return float64(v.i)
}

func (v exprVal) asInt() int64 {
	if v.kind == ekFloat || v.kind == ekDouble {
		return int64(v.f)
	}
	return v.i
}

func (v exprVal) boolean() bool { return v.i != 0 }

func (v exprVal) sig() string {
	switch v.kind {
	case ekBoolean:
		return "Z"
	case ekByte:
		return "B"
	case ekChar:
		return "C"
	case ekShort:
		return "S"
	case ekInt:
		return "I"
	case ekLong:
		return "J"
	case ekFloat:
		return "F"
	case ekDouble:
		return "D"
	case ekString:
		return "Ljava/lang/String;"
	case ekArray:
		if v.name != "" {
			return elemSigOfType(v.name)
		}
		return "Ljava/lang/Object;"
	case ekNull:
		return "Ljava/lang/Object;"
	}
	if v.name != "" {
		return "L" + strings.ReplaceAll(v.name, ".", "/") + ";"
	}
	return "Ljava/lang/Object;"
}

func (v exprVal) toValue() jdwpValue {
	switch v.kind {
	case ekBoolean, ekByte, ekChar, ekShort, ekInt, ekLong:
		return jdwpValue{tag: tagOfKind(v.kind), i: v.i}
	case ekFloat, ekDouble:
		return jdwpValue{tag: tagOfKind(v.kind), f: v.f}
	case ekString:
		return jdwpValue{tag: tagString, obj: v.obj}
	}
	return jdwpValue{tag: tagObject, obj: v.obj}
}

func tagOfKind(k exprKind) byte {
	switch k {
	case ekBoolean:
		return tagBoolean
	case ekByte:
		return tagByte
	case ekChar:
		return tagChar
	case ekShort:
		return tagShort
	case ekInt:
		return tagInt
	case ekLong:
		return tagLong
	case ekFloat:
		return tagFloat
	case ekDouble:
		return tagDouble
	}
	return tagObject
}

func (v exprVal) text(d *debugger) string {
	switch v.kind {
	case ekVoid:
		return "void"
	case ekString:
		return strconv.Quote(v.s)
	case ekNull:
		return "null"
	case ekBoolean:
		if v.i != 0 {
			return "true"
		}
		return "false"
	case ekChar:
		return "'" + string(rune(v.i)) + "'"
	case ekFloat:
		return strconv.FormatFloat(v.f, 'g', -1, 32) + "f"
	case ekDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case ekArray, ekObject:
		if v.obj == 0 {
			return "null"
		}
		if v.kind == ekArray {
			return d.arrayLabel(v.obj)
		}
		return d.objectLabel(v.obj)
	}
	return strconv.FormatInt(v.i, 10)
}

func (v exprVal) rawText(d *debugger) string {
	if v.kind == ekString {
		return v.s
	}
	return v.text(d)
}

type exprScope struct {
	d      *debugger
	thread int64
	frame  int64
	index  int
	ready  bool
	locals map[string]exprVal
	fields map[string]exprVal
	static map[string]exprVal
	this   exprVal
	class  int64
	err    error
}

func (sc *exprScope) init() {
	if sc.ready {
		return
	}
	sc.ready = true
	d := sc.d
	frames, err := d.conn.frames(sc.thread)
	if err != nil || len(frames) == 0 {
		sc.err = err
		return
	}
	if sc.index >= len(frames) {
		sc.index = 0
	}
	f := frames[sc.index]
	sc.frame = f.id
	sc.class = f.loc.class
	sc.locals = map[string]exprVal{}
	sc.fields = map[string]exprVal{}
	sc.static = map[string]exprVal{}
	if slots, err := d.conn.variableTable(f.loc.class, f.loc.method); err == nil {
		nums := make([]int, 0, len(slots))
		tags := make([]byte, 0, len(slots))
		for _, s := range slots {
			nums = append(nums, s.slot)
			tags = append(tags, tagFromSig(s.sig))
		}
		if values, err := d.conn.localValues(sc.thread, f.id, nums, tags); err == nil {
			for i, s := range slots {
				if i >= len(values) {
					break
				}
				sc.locals[s.name] = valueToExpr(d, values[i])
			}
		}
	}
	if this, err := d.conn.thisObject(sc.thread, f.id); err == nil && this.obj != 0 {
		sc.this = valueToExpr(d, this)
		for _, v := range d.fieldsOf(this.obj, f.loc.class, "") {
			sc.fields[v.name] = sc.fieldVal(v)
		}
	}
	for _, v := range d.staticFieldsOf(f.loc.class, "") {
		sc.static[v.name] = sc.fieldVal(v)
	}
}

func (sc *exprScope) fieldVal(v *debugVar) exprVal {
	out := exprVal{obj: v.objID, name: v.typeName, i: v.numi, f: v.numf}
	switch v.tag {
	case tagInt:
		out.kind = ekInt
	case tagLong:
		out.kind = ekLong
	case tagDouble:
		out.kind = ekDouble
	case tagFloat:
		out.kind = ekFloat
	case tagBoolean:
		out.kind = ekBoolean
	case tagChar:
		out.kind = ekChar
	case tagByte:
		out.kind = ekByte
	case tagShort:
		out.kind = ekShort
	case tagString:
		out.kind = ekString
		out.s = strings.Trim(v.value, "\"")
	case tagArray:
		out.kind = ekArray
	default:
		out.kind = ekObject
	}
	if v.tag == tagString {
		out.obj = v.objID
	}
	return out
}

func valueToExpr(d *debugger, v jdwpValue) exprVal {
	out := exprVal{}
	switch v.tag {
	case tagInt:
		out.kind = ekInt
		out.i = v.i
	case tagLong:
		out.kind = ekLong
		out.i = v.i
	case tagDouble:
		out.kind = ekDouble
		out.f = v.f
	case tagFloat:
		out.kind = ekFloat
		out.f = v.f
	case tagBoolean:
		out.kind = ekBoolean
		out.i = v.i
	case tagChar:
		out.kind = ekChar
		out.i = v.i
	case tagByte:
		out.kind = ekByte
		out.i = v.i
	case tagShort:
		out.kind = ekShort
		out.i = v.i
	case tagString:
		if v.obj == 0 {
			out.kind = ekNull
		} else {
			out.kind = ekString
			out.obj = v.obj
			if s, err := d.conn.stringValue(v.obj); err == nil {
				out.s = s
			}
		}
	case tagArray:
		if v.obj == 0 {
			out.kind = ekNull
		} else {
			out.kind = ekArray
			out.obj = v.obj
			out.name = d.typeNameOfObject(v.obj)
		}
	case tagObject:
		if v.obj == 0 {
			out.kind = ekNull
		} else {
			out.kind = ekObject
			out.obj = v.obj
			out.name = d.typeNameOfObject(v.obj)
		}
	default:
		out.kind = ekObject
		out.obj = v.obj
	}
	return out
}

func evalExpression(expr string, sc *exprScope) (string, error) {
	p := &exprParser{toks: lexExpr(expr), sc: sc}
	v, err := p.parse()
	if err != nil {
		return "", err
	}
	return v.text(sc.d), nil
}

type exprTokKind int

const (
	tkEOF exprTokKind = iota
	tkIdent
	tkNumber
	tkString
	tkChar
	tkOp
)

type exprTok struct {
	kind exprTokKind
	text string
}

func lexExpr(src string) []exprTok {
	var out []exprTok
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			out = append(out, exprTok{kind: tkIdent, text: src[i:j]})
			i = j
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9':
			j := i
			for j < len(src) && (isDigit(src[j]) || src[j] == '.' || src[j] == 'x' || src[j] == 'X' ||
				(src[j] >= 'a' && src[j] <= 'f') || (src[j] >= 'A' && src[j] <= 'F') || src[j] == '_') {
				j++
			}
			if j < len(src) && (src[j] == 'l' || src[j] == 'L' || src[j] == 'f' || src[j] == 'F' || src[j] == 'd' || src[j] == 'D') {
				j++
			}
			out = append(out, exprTok{kind: tkNumber, text: src[i:j]})
			i = j
		case c == '"':
			j := i + 1
			var sb strings.Builder
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' && j+1 < len(src) {
					switch src[j+1] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					case 'r':
						sb.WriteByte('\r')
					default:
						sb.WriteByte(src[j+1])
					}
					j += 2
					continue
				}
				sb.WriteByte(src[j])
				j++
			}
			if j >= len(src) {
				return append(out, exprTok{kind: tkEOF})
			}
			out = append(out, exprTok{kind: tkString, text: sb.String()})
			i = j + 1
		case c == '\'':
			if i+2 < len(src) && src[i+2] == '\'' {
				out = append(out, exprTok{kind: tkChar, text: string(rune(src[i+1]))})
				i += 3
			} else {
				return append(out, exprTok{kind: tkEOF})
			}
		default:
			two := ""
			if i+1 < len(src) {
				two = src[i : i+2]
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||", "++", "--":
				out = append(out, exprTok{kind: tkOp, text: two})
				i += 2
			default:
				out = append(out, exprTok{kind: tkOp, text: string(rune(c))})
				i++
			}
		}
	}
	out = append(out, exprTok{kind: tkEOF})
	return out
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

type exprParser struct {
	toks []exprTok
	pos  int
	sc   *exprScope
}

func (p *exprParser) peek() exprTok { return p.toks[p.pos] }

func (p *exprParser) next() exprTok {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *exprParser) isOp(s string) bool {
	t := p.peek()
	return t.kind == tkOp && t.text == s
}

func (p *exprParser) accept(s string) bool {
	if p.isOp(s) {
		p.next()
		return true
	}
	return false
}

func (p *exprParser) parse() (exprVal, error) {
	v, err := p.ternary()
	if err != nil {
		return v, err
	}
	if p.peek().kind != tkEOF {
		return v, fmt.Errorf("表达式末尾多余的内容")
	}
	return v, nil
}

func (p *exprParser) ternary() (exprVal, error) {
	v, err := p.or()
	if err != nil {
		return v, err
	}
	if p.accept("?") {
		a, err := p.ternary()
		if err != nil {
			return a, err
		}
		if !p.accept(":") {
			return a, fmt.Errorf("缺少 :")
		}
		b, err := p.ternary()
		if err != nil {
			return b, err
		}
		if v.boolean() {
			return a, nil
		}
		return b, nil
	}
	return v, nil
}

func (p *exprParser) or() (exprVal, error) {
	v, err := p.and()
	if err != nil {
		return v, err
	}
	for p.isOp("||") {
		p.next()
		r, err := p.and()
		if err != nil {
			return r, err
		}
		v = exprVal{kind: ekBoolean, i: boolToInt(v.boolean() || r.boolean())}
	}
	return v, nil
}

func (p *exprParser) and() (exprVal, error) {
	v, err := p.equality()
	if err != nil {
		return v, err
	}
	for p.isOp("&&") {
		p.next()
		r, err := p.equality()
		if err != nil {
			return r, err
		}
		v = exprVal{kind: ekBoolean, i: boolToInt(v.boolean() && r.boolean())}
	}
	return v, nil
}

func (p *exprParser) equality() (exprVal, error) {
	v, err := p.relational()
	if err != nil {
		return v, err
	}
	for p.isOp("==") || p.isOp("!=") {
		op := p.next().text
		r, err := p.relational()
		if err != nil {
			return r, err
		}
		v = compareExpr(v, r, op)
	}
	return v, nil
}

func compareExpr(a, b exprVal, op string) exprVal {
	eq := false
	switch {
	case a.kind == ekNull || b.kind == ekNull:
		eq = (a.kind == ekNull && b.kind == ekNull) || (a.obj == 0 && b.obj == 0 && a.obj == b.obj)
		if a.kind == ekNull && b.kind == ekNull {
			eq = true
		} else if a.kind == ekNull || b.kind == ekNull {
			eq = a.obj == b.obj
		}
	case a.numeric() && b.numeric():
		eq = a.asFloat() == b.asFloat()
	case a.kind == ekString && b.kind == ekString:
		eq = a.s == b.s
	default:
		eq = a.obj == b.obj
	}
	if op == "!=" {
		eq = !eq
	}
	return exprVal{kind: ekBoolean, i: boolToInt(eq)}
}

func (p *exprParser) relational() (exprVal, error) {
	v, err := p.additive()
	if err != nil {
		return v, err
	}
	for p.isOp("<") || p.isOp(">") || p.isOp("<=") || p.isOp(">=") {
		op := p.next().text
		r, err := p.additive()
		if err != nil {
			return r, err
		}
		res := false
		switch {
		case v.kind == ekString && r.kind == ekString:
			c := strings.Compare(v.s, r.s)
			res = orderHolds(c, op)
		case v.numeric() && r.numeric():
			a, b := v.asFloat(), r.asFloat()
			res = orderHoldsF(a, b, op)
		default:
			return exprVal{}, fmt.Errorf("无法比较该类型")
		}
		v = exprVal{kind: ekBoolean, i: boolToInt(res)}
	}
	return v, nil
}

func orderHolds(c int, op string) bool {
	switch op {
	case "<":
		return c < 0
	case ">":
		return c > 0
	case "<=":
		return c <= 0
	}
	return c >= 0
}

func orderHoldsF(a, b float64, op string) bool {
	switch op {
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	}
	return a >= b
}

func (p *exprParser) additive() (exprVal, error) {
	v, err := p.multiplicative()
	if err != nil {
		return v, err
	}
	for p.isOp("+") || p.isOp("-") {
		op := p.next().text
		r, err := p.multiplicative()
		if err != nil {
			return r, err
		}
		if op == "+" && (v.kind == ekString || r.kind == ekString) {
			v = exprVal{kind: ekString, s: v.rawText(p.sc.d) + r.rawText(p.sc.d)}
			continue
		}
		if !v.numeric() || !r.numeric() {
			return exprVal{}, fmt.Errorf("无法对非数值类型进行 %s 运算", op)
		}
		v = arithExpr(v, r, op)
	}
	return v, nil
}

func arithExpr(a, b exprVal, op string) exprVal {
	kind := widerKind(a.kind, b.kind)
	if kind == ekDouble || kind == ekFloat {
		x, y := a.asFloat(), b.asFloat()
		switch op {
		case "+":
			return exprVal{kind: kind, f: x + y}
		case "-":
			return exprVal{kind: kind, f: x - y}
		}
		if y == 0 {
			return exprVal{kind: ekDouble, f: 0}
		}
		switch op {
		case "*":
			return exprVal{kind: kind, f: x * y}
		case "/":
			return exprVal{kind: kind, f: x / y}
		}
		return exprVal{kind: kind, f: float64(int64(x) % int64(y))}
	}
	x, y := a.asInt(), b.asInt()
	switch op {
	case "+":
		return exprVal{kind: kind, i: x + y}
	case "-":
		return exprVal{kind: kind, i: x - y}
	case "*":
		return exprVal{kind: kind, i: x * y}
	}
	if y == 0 {
		return exprVal{kind: kind, i: 0}
	}
	if op == "/" {
		return exprVal{kind: kind, i: x / y}
	}
	return exprVal{kind: kind, i: x % y}
}

func widerKind(a, b exprKind) exprKind {
	rank := map[exprKind]int{
		ekByte: 1, ekShort: 2, ekChar: 2, ekInt: 3, ekLong: 4, ekFloat: 5, ekDouble: 6,
	}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}

func (p *exprParser) multiplicative() (exprVal, error) {
	v, err := p.unary()
	if err != nil {
		return v, err
	}
	for p.isOp("*") || p.isOp("/") || p.isOp("%") {
		op := p.next().text
		r, err := p.unary()
		if err != nil {
			return r, err
		}
		if !v.numeric() || !r.numeric() {
			return exprVal{}, fmt.Errorf("无法对非数值类型进行 %s 运算", op)
		}
		v = arithExpr(v, r, op)
	}
	return v, nil
}

func (p *exprParser) unary() (exprVal, error) {
	if p.isOp("!") {
		p.next()
		v, err := p.unary()
		if err != nil {
			return v, err
		}
		return exprVal{kind: ekBoolean, i: boolToInt(!v.boolean())}, nil
	}
	if p.isOp("-") {
		p.next()
		v, err := p.unary()
		if err != nil {
			return v, err
		}
		if !v.numeric() {
			return exprVal{}, fmt.Errorf("无法对非数值类型取负")
		}
		if v.kind == ekFloat || v.kind == ekDouble {
			return exprVal{kind: v.kind, f: -v.f}, nil
		}
		return exprVal{kind: v.kind, i: -v.i}, nil
	}
	if p.isOp("+") {
		p.next()
		return p.unary()
	}
	return p.postfix()
}

func (p *exprParser) postfix() (exprVal, error) {
	v, err := p.primary()
	if err != nil {
		return v, err
	}
	for {
		if p.isOp(".") {
			p.next()
			t := p.peek()
			if t.kind != tkIdent {
				return v, fmt.Errorf("成员名称无效")
			}
			p.next()
			if p.isOp("(") {
				args, err := p.arguments()
				if err != nil {
					return v, err
				}
				v, err = p.callMethod(v, t.text, args)
				if err != nil {
					return v, err
				}
				continue
			}
			v, err = p.member(v, t.text)
			if err != nil {
				return v, err
			}
			continue
		}
		if p.isOp("(") {
			args, err := p.arguments()
			if err != nil {
				return v, err
			}
			v, err = p.callFunction(v, args)
			if err != nil {
				return v, err
			}
			continue
		}
		if p.isOp("[") {
			p.next()
			idx, err := p.ternary()
			if err != nil {
				return v, err
			}
			if !p.accept("]") {
				return v, fmt.Errorf("缺少 ]")
			}
			v, err = p.element(v, idx)
			if err != nil {
				return v, err
			}
			continue
		}
		return p.resolveIdent(v)
	}
}

func (p *exprParser) resolveIdent(v exprVal) (exprVal, error) {
	if v.ident == "" {
		return v, nil
	}
	sc := p.sc
	sc.init()
	if sc.err != nil {
		return v, sc.err
	}
	if lv, ok := sc.locals[v.ident]; ok {
		return lv, nil
	}
	if fv, ok := sc.fields[v.ident]; ok {
		return fv, nil
	}
	if sv, ok := sc.static[v.ident]; ok {
		return sv, nil
	}
	if v.ident == "this" && sc.this.kind != ekVoid {
		return sc.this, nil
	}
	return v, fmt.Errorf("找不到变量 %s", v.ident)
}

func (p *exprParser) resolveArgs(args []exprVal) ([]exprVal, error) {
	out := make([]exprVal, 0, len(args))
	for _, a := range args {
		v, err := p.resolveIdent(a)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (p *exprParser) member(base exprVal, name string) (exprVal, error) {
	d := p.sc.d
	base, err := p.resolveIdent(base)
	if err != nil {
		if base.ident != "" {
			if typeID := d.classOfName(base.ident); typeID != 0 {
				return d.staticField(typeID, name)
			}
		}
		return base, fmt.Errorf("找不到 %s", name)
	}
	if base.kind == ekNull {
		return base, fmt.Errorf("对 null 访问成员 %s", name)
	}
	if base.kind == ekArray && name == "length" {
		return exprVal{kind: ekInt, i: int64(d.conn.arrayLength(base.obj))}, nil
	}
	if base.kind != ekObject && base.kind != ekArray && base.kind != ekString {
		return base, fmt.Errorf("该类型没有成员 %s", name)
	}
	return d.instanceField(base.obj, name)
}

func (p *exprParser) element(base, idx exprVal) (exprVal, error) {
	d := p.sc.d
	base, err := p.resolveIdent(base)
	if err != nil {
		return base, err
	}
	if base.kind != ekArray {
		return base, fmt.Errorf("只能对数组取下标")
	}
	i := int(idx.asInt())
	n := d.conn.arrayLength(base.obj)
	if i < 0 || i >= n {
		return base, fmt.Errorf("数组下标越界")
	}
	values, err := d.conn.arrayValues(base.obj, i, 1, elemSigOfType(base.name))
	if err != nil || len(values) == 0 {
		return base, fmt.Errorf("无法读取数组元素")
	}
	return valueToExpr(d, values[0]), nil
}

func (p *exprParser) callMethod(base exprVal, name string, args []exprVal) (exprVal, error) {
	d := p.sc.d
	vals, err := p.resolveArgs(args)
	if err != nil {
		return base, err
	}
	argv := make([]jdwpValue, 0, len(vals))
	for _, v := range vals {
		argv = append(argv, v.toValue())
	}
	resolved, resErr := p.resolveIdent(base)
	if resErr != nil {
		if typeID := d.classOfName(base.ident); typeID != 0 {
			m := d.findMethod(typeID, name, vals)
			if m == nil {
				return base, fmt.Errorf("找不到方法 %s.%s", base.ident, name)
			}
			ret, exc := d.conn.invokeStatic(p.sc.thread, typeID, m.id, argv)
			if exc != 0 {
				return base, fmt.Errorf("方法 %s 抛出异常", name)
			}
			return valueToExpr(d, ret), nil
		}
		if v, err := p.callUnqualifiedNamed(name, vals); err == nil {
			return v, nil
		}
		return base, fmt.Errorf("找不到 %s.%s", base.ident, name)
	}
	base = resolved
	if base.kind == ekNull {
		return base, fmt.Errorf("对 null 调用方法 %s", name)
	}
	if base.kind != ekObject && base.kind != ekArray && base.kind != ekString {
		return base, fmt.Errorf("该类型没有方法 %s", name)
	}
	typeID := d.conn.objectType(base.obj)
	if typeID == 0 {
		return base, fmt.Errorf("无法确定对象类型")
	}
	m := d.findMethod(typeID, name, vals)
	if m == nil {
		return base, fmt.Errorf("找不到方法 %s", name)
	}
	ret, exc := d.conn.invokeMethod(base.obj, p.sc.thread, typeID, m.id, argv)
	if exc != 0 {
		return base, fmt.Errorf("方法 %s 抛出异常", name)
	}
	return valueToExpr(d, ret), nil
}

func (p *exprParser) callFunction(base exprVal, args []exprVal) (exprVal, error) {
	if base.ident == "" {
		return base, fmt.Errorf("只能调用具名方法")
	}
	return p.callUnqualifiedNamed(base.ident, args)
}

func (p *exprParser) callUnqualified(name string, vals []exprVal, argv []jdwpValue) (exprVal, bool) {
	v, err := p.callUnqualifiedNamed(name, vals)
	if err != nil {
		return exprVal{}, false
	}
	return v, true
}

func (p *exprParser) callUnqualifiedNamed(name string, args []exprVal) (exprVal, error) {
	d := p.sc.d
	sc := p.sc
	sc.init()
	if sc.err != nil {
		return exprVal{}, sc.err
	}
	vals, err := p.resolveArgs(args)
	if err != nil {
		return exprVal{}, err
	}
	argv := make([]jdwpValue, 0, len(vals))
	for _, v := range vals {
		argv = append(argv, v.toValue())
	}
	if m := d.findMethod(sc.class, name, vals); m != nil && m.static {
		ret, exc := d.conn.invokeStatic(sc.thread, sc.class, m.id, argv)
		if exc != 0 {
			return exprVal{}, fmt.Errorf("方法 %s 抛出异常", name)
		}
		return valueToExpr(d, ret), nil
	}
	if sc.this.obj != 0 {
		typeID := d.conn.objectType(sc.this.obj)
		if m := d.findMethod(typeID, name, vals); m != nil {
			ret, exc := d.conn.invokeMethod(sc.this.obj, sc.thread, typeID, m.id, argv)
			if exc != 0 {
				return exprVal{}, fmt.Errorf("方法 %s 抛出异常", name)
			}
			return valueToExpr(d, ret), nil
		}
	}
	return exprVal{}, fmt.Errorf("找不到方法 %s", name)
}

func (d *debugger) classOfName(name string) int64 {
	sig := "L" + strings.ReplaceAll(name, ".", "/") + ";"
	if id := d.lookupClass(sig); id != 0 {
		return id
	}
	if !strings.Contains(name, ".") {
		if id := d.lookupClass("Ljava/lang/" + name + ";"); id != 0 {
			return id
		}
	}
	return 0
}

func (d *debugger) lookupClass(sig string) int64 {
	d.mu.Lock()
	if id, ok := d.sigToType[sig]; ok {
		d.mu.Unlock()
		return id
	}
	d.mu.Unlock()
	ids, err := d.conn.classesBySignature(sig)
	if err != nil || len(ids) == 0 {
		return 0
	}
	d.mu.Lock()
	d.sigToType[sig] = ids[0]
	d.mu.Unlock()
	return ids[0]
}

func (d *debugger) instanceField(objID int64, name string) (exprVal, error) {
	typeID := d.conn.objectType(objID)
	if typeID == 0 {
		return exprVal{}, fmt.Errorf("无法确定对象类型")
	}
	fields, err := d.conn.fields(typeID)
	if err != nil {
		return exprVal{}, err
	}
	var ids []int64
	var found *jdwpField
	for i := range fields {
		if fields[i].name == name && !fields[i].static {
			found = &fields[i]
			ids = append(ids, fields[i].id)
		}
	}
	if found == nil {
		return exprVal{}, fmt.Errorf("找不到字段 %s", name)
	}
	values, err := d.conn.fieldValues(objID, ids)
	if err != nil || len(values) == 0 {
		return exprVal{}, fmt.Errorf("无法读取字段 %s", name)
	}
	return valueToExpr(d, values[0]), nil
}

func (d *debugger) staticField(typeID int64, name string) (exprVal, error) {
	fields, err := d.conn.fields(typeID)
	if err != nil {
		return exprVal{}, err
	}
	var ids []int64
	var found *jdwpField
	for i := range fields {
		if fields[i].name == name && fields[i].static {
			found = &fields[i]
			ids = append(ids, fields[i].id)
		}
	}
	if found == nil {
		return exprVal{}, fmt.Errorf("找不到静态字段 %s", name)
	}
	values, err := d.conn.staticValues(typeID, ids)
	if err != nil || len(values) == 0 {
		return exprVal{}, fmt.Errorf("无法读取静态字段 %s", name)
	}
	return valueToExpr(d, values[0]), nil
}

func (d *debugger) findMethod(typeID int64, name string, args []exprVal) *jdwpMethod {
	want := make([]string, 0, len(args))
	for _, a := range args {
		want = append(want, a.sig())
	}
	for i := 0; i < 12 && typeID != 0; i++ {
		ms, err := d.conn.methods(typeID)
		if err == nil {
			var compatible *jdwpMethod
			for j := range ms {
				m := &ms[j]
				if m.name != name || !strings.HasPrefix(m.sig, "(") {
					continue
				}
				params := methodParams(m.sig)
				if len(params) != len(args) {
					continue
				}
				if slices.Equal(params, want) {
					return m
				}
				if compatible == nil && methodAccepts(params, args) {
					compatible = m
				}
			}
			if compatible != nil {
				return compatible
			}
		}
		typeID = d.conn.superclass(typeID)
	}
	return nil
}

func methodAccepts(params []string, args []exprVal) bool {
	for i, p := range params {
		v := args[i]
		ref := len(p) > 0 && (p[0] == 'L' || p[0] == '[')
		switch v.kind {
		case ekNull:
			if !ref {
				return false
			}
		case ekString, ekObject, ekArray:
			if !ref {
				return false
			}
		default:
			if p != v.sig() {
				return false
			}
		}
	}
	return true
}

func methodParams(sig string) []string {
	if !strings.HasPrefix(sig, "(") {
		return nil
	}
	var out []string
	for i := 1; i < len(sig); i++ {
		switch sig[i] {
		case ')':
			return out
		case '[':
			j := i
			for j < len(sig) && sig[j] == '[' {
				j++
			}
			if j < len(sig) && sig[j] == 'L' {
				k := strings.IndexByte(sig[j:], ';')
				if k < 0 {
					return out
				}
				out = append(out, sig[i:j+k+1])
				i = j + k
				continue
			}
			out = append(out, sig[i:j+1])
			i = j
		case 'L':
			k := strings.IndexByte(sig[i:], ';')
			if k < 0 {
				return out
			}
			out = append(out, sig[i:i+k+1])
			i += k
		default:
			out = append(out, sig[i:i+1])
		}
	}
	return out
}

func (p *exprParser) arguments() ([]exprVal, error) {
	if !p.accept("(") {
		return nil, fmt.Errorf("缺少 (")
	}
	var out []exprVal
	if p.isOp(")") {
		p.next()
		return out, nil
	}
	for {
		v, err := p.ternary()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if p.accept(",") {
			continue
		}
		if p.accept(")") {
			return out, nil
		}
		return nil, fmt.Errorf("参数列表缺少 )")
	}
}

func (p *exprParser) primary() (exprVal, error) {
	t := p.peek()
	switch t.kind {
	case tkNumber:
		p.next()
		return numberExpr(t.text)
	case tkString:
		p.next()
		return exprVal{kind: ekString, s: t.text}, nil
	case tkChar:
		p.next()
		for _, r := range t.text {
			return exprVal{kind: ekChar, i: int64(r)}, nil
		}
		return exprVal{kind: ekChar}, nil
	case tkIdent:
		switch t.text {
		case "true":
			p.next()
			return exprVal{kind: ekBoolean, i: 1}, nil
		case "false":
			p.next()
			return exprVal{kind: ekBoolean}, nil
		case "null":
			p.next()
			return exprVal{kind: ekNull}, nil
		case "new":
			return exprVal{}, fmt.Errorf("暂不支持 new 表达式")
		}
		p.next()
		return exprVal{kind: ekObject, name: t.text, ident: t.text}, nil
	case tkOp:
		if t.text == "(" {
			save := p.pos
			p.next()
			if v, ok, err := p.tryCast(); ok {
				return v, err
			}
			p.pos = save
			p.next()
			v, err := p.ternary()
			if err != nil {
				return v, err
			}
			if !p.accept(")") {
				return v, fmt.Errorf("缺少 )")
			}
			return v, nil
		}
	}
	return exprVal{}, fmt.Errorf("无法解析表达式")
}

func (p *exprParser) tryCast() (exprVal, bool, error) {
	if p.peek().kind != tkIdent {
		return exprVal{}, false, nil
	}
	var parts []string
	for p.peek().kind == tkIdent {
		parts = append(parts, p.next().text)
		if !p.isOp(".") {
			break
		}
		p.next()
	}
	for p.isOp("[") {
		p.next()
		if !p.accept("]") {
			return exprVal{}, false, nil
		}
		parts = append(parts, "[]")
	}
	if !p.isOp(")") {
		return exprVal{}, false, nil
	}
	p.next()
	v, err := p.unary()
	if err != nil {
		return v, true, err
	}
	return v, true, nil
}

func numberExpr(text string) (exprVal, error) {
	s := text
	if strings.ContainsAny(s, ".eE") && !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(s, "f"), "d"), 64)
		if err != nil {
			return exprVal{}, fmt.Errorf("数字字面量无效")
		}
		kind := ekDouble
		if strings.HasSuffix(s, "f") || strings.HasSuffix(s, "F") {
			kind = ekFloat
		}
		return exprVal{kind: kind, f: f}, nil
	}
	base := 10
	clean := s
	if strings.HasPrefix(clean, "0x") || strings.HasPrefix(clean, "0X") {
		base, clean = 16, clean[2:]
	} else if strings.HasPrefix(clean, "0b") || strings.HasPrefix(clean, "0B") {
		base, clean = 2, clean[2:]
	}
	long := false
	if strings.HasSuffix(clean, "l") || strings.HasSuffix(clean, "L") {
		long = true
		clean = clean[:len(clean)-1]
	}
	clean = strings.ReplaceAll(clean, "_", "")
	i, err := strconv.ParseInt(clean, base, 64)
	if err != nil {
		return exprVal{}, fmt.Errorf("数字字面量无效")
	}
	if long {
		return exprVal{kind: ekLong, i: i}, nil
	}
	if i > 2147483647 || i < -2147483648 {
		return exprVal{kind: ekLong, i: i}, nil
	}
	return exprVal{kind: ekInt, i: i}, nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
