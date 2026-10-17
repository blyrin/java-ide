package main

type jNode interface {
	rng() jRange
}

type jFile struct {
	Path    string
	Pkg     *jPkg
	Imports []*jImport
	Types   []*jTypeDecl
	Range   jRange
}

func (f *jFile) rng() jRange { return f.Range }

type jPkg struct {
	Name  string
	Range jRange
}

func (p *jPkg) rng() jRange { return p.Range }

type jImport struct {
	Name     string
	IsStatic bool
	IsStar   bool
	Range    jRange
}

func (i *jImport) rng() jRange { return i.Range }

type jMember interface {
	jNode
	member()
}

type jTypeDecl struct {
	Kind        jKind
	Name        string
	Modifiers   []string
	Annotations []*jAnno
	Generics    []*jTypeParam
	Extends     []string
	Implements  []string
	Permits     []string
	Members     []jMember
	Range       jRange
	NameRange   jRange
	Doc         string
}

func (t *jTypeDecl) rng() jRange { return t.Range }
func (t *jTypeDecl) member()     {}

type jFieldDecl struct {
	Name        string
	Type        string
	Modifiers   []string
	Annotations []*jAnno
	IsEnumConst bool
	Range       jRange
	NameRange   jRange
	Doc         string
}

func (f *jFieldDecl) rng() jRange { return f.Range }
func (f *jFieldDecl) member()     {}

type jMethodDecl struct {
	Name        string
	ReturnType  string
	Modifiers   []string
	Annotations []*jAnno
	Generics    []*jTypeParam
	Parameters  []*jParam
	Exceptions  []string
	Default     string
	Body        *jBlock
	Range       jRange
	NameRange   jRange
	Doc         string
}

func (m *jMethodDecl) rng() jRange { return m.Range }
func (m *jMethodDecl) member()     {}

type jCtorDecl struct {
	Name        string
	Modifiers   []string
	Annotations []*jAnno
	Parameters  []*jParam
	Exceptions  []string
	Body        *jBlock
	Range       jRange
	NameRange   jRange
	Doc         string
}

func (c *jCtorDecl) rng() jRange { return c.Range }
func (c *jCtorDecl) member()     {}

type jInitBlock struct {
	Static bool
	Body   *jBlock
	Range  jRange
	Doc    string
}

func (b *jInitBlock) rng() jRange { return b.Range }
func (b *jInitBlock) member()     {}

type jBlock struct {
	Locals []jNode
	Range  jRange
}

func (b *jBlock) rng() jRange { return b.Range }

type jLocalVar struct {
	Name      string
	Type      string
	Modifiers []string
	Range     jRange
	NameRange jRange
}

func (v *jLocalVar) rng() jRange { return v.Range }
