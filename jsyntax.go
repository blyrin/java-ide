package main

type jPos struct {
	Line int
	Col  int
}

type jRange struct {
	Start jPos
	End   jPos
}

func (r jRange) contains(p jPos) bool {
	if p.Line < r.Start.Line || p.Line > r.End.Line {
		return false
	}
	if p.Line == r.Start.Line && p.Col < r.Start.Col {
		return false
	}
	if p.Line == r.End.Line && p.Col > r.End.Col {
		return false
	}
	return true
}

type jKind int

const (
	jKindClass jKind = iota
	jKindInterface
	jKindEnum
	jKindRecord
	jKindAnnotation
)

func (k jKind) String() string {
	switch k {
	case jKindInterface:
		return "interface"
	case jKindEnum:
		return "enum"
	case jKindRecord:
		return "record"
	case jKindAnnotation:
		return "@interface"
	default:
		return "class"
	}
}

type jParam struct {
	Name        string
	Type        string
	Annotations []string
	Modifiers   []string
	VarArgs     bool
	Range       jRange
}

func (p *jParam) text() string {
	t := p.Type
	if p.VarArgs {
		t += "..."
	}
	if p.Name == "" {
		return t
	}
	return t + " " + p.Name
}

type jAnno struct {
	Name  string
	Range jRange
}

type jTypeParam struct {
	Name   string
	Bounds []string
	Range  jRange
}
