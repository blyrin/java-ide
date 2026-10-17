package main

type jEventType string

const (
	jEvStartFile   jEventType = "StartFile"
	jEvEndFile     jEventType = "EndFile"
	jEvPackageDecl jEventType = "PackageDecl"
	jEvImportDecl  jEventType = "ImportDecl"
	jEvTypeDecl    jEventType = "TypeDecl"
	jEvFieldDecl   jEventType = "FieldDecl"
	jEvMethodDecl  jEventType = "MethodDecl"
	jEvCtorDecl    jEventType = "ConstructorDecl"
	jEvInitBlock   jEventType = "InitBlock"
	jEvScopeEnter  jEventType = "ScopeEnter"
	jEvScopeLeave  jEventType = "ScopeLeave"
	jEvLocalDecl   jEventType = "LocalDecl"
	jEvStartDelta  jEventType = "StartDeltaScope"
	jEvEndDelta    jEventType = "EndDeltaScope"
	jEvParseError  jEventType = "ParseError"
)

type jEvent interface {
	Type() jEventType
	GetRange() jRange
}

type jEventSink interface {
	Emit(evt jEvent) error
}

type jPolicy uint32

const (
	jPolicyDeclarations jPolicy = 1 << iota
	jPolicyOccurrences
)

type jStartFileEvent struct {
	SourceID string
}

func (e jStartFileEvent) Type() jEventType { return jEvStartFile }
func (e jStartFileEvent) GetRange() jRange { return jRange{} }

type jEndFileEvent struct {
	SourceID string
}

func (e jEndFileEvent) Type() jEventType { return jEvEndFile }
func (e jEndFileEvent) GetRange() jRange { return jRange{} }

type jPackageDeclEvent struct {
	SourceID string
	Name     string
	Range    jRange
}

func (e jPackageDeclEvent) Type() jEventType { return jEvPackageDecl }
func (e jPackageDeclEvent) GetRange() jRange { return e.Range }

type jImportDeclEvent struct {
	SourceID string
	Name     string
	IsStatic bool
	IsStar   bool
	Range    jRange
}

func (e jImportDeclEvent) Type() jEventType { return jEvImportDecl }
func (e jImportDeclEvent) GetRange() jRange { return e.Range }

type jTypeDeclEvent struct {
	SourceID    string
	Kind        jKind
	Name        string
	Modifiers   []string
	Annotations []string
	Range       jRange
	Doc         string
}

func (e jTypeDeclEvent) Type() jEventType { return jEvTypeDecl }
func (e jTypeDeclEvent) GetRange() jRange { return e.Range }

type jFieldDeclEvent struct {
	SourceID    string
	Name        string
	DataType    string
	Modifiers   []string
	Annotations []string
	IsEnumConst bool
	Range       jRange
	Doc         string
}

func (e jFieldDeclEvent) Type() jEventType { return jEvFieldDecl }
func (e jFieldDeclEvent) GetRange() jRange { return e.Range }

type jMethodDeclEvent struct {
	SourceID    string
	Name        string
	ReturnType  string
	Modifiers   []string
	Annotations []string
	Parameters  []*jParam
	Exceptions  []string
	Range       jRange
	Doc         string
}

func (e jMethodDeclEvent) Type() jEventType { return jEvMethodDecl }
func (e jMethodDeclEvent) GetRange() jRange { return e.Range }

type jCtorDeclEvent struct {
	SourceID    string
	Name        string
	Modifiers   []string
	Annotations []string
	Parameters  []*jParam
	Exceptions  []string
	Range       jRange
	Doc         string
}

func (e jCtorDeclEvent) Type() jEventType { return jEvCtorDecl }
func (e jCtorDeclEvent) GetRange() jRange { return e.Range }

type jInitBlockEvent struct {
	SourceID string
	Static   bool
	Range    jRange
}

func (e jInitBlockEvent) Type() jEventType { return jEvInitBlock }
func (e jInitBlockEvent) GetRange() jRange { return e.Range }

type jScopeEnterEvent struct {
	Range jRange
}

func (e jScopeEnterEvent) Type() jEventType { return jEvScopeEnter }
func (e jScopeEnterEvent) GetRange() jRange { return e.Range }

type jScopeLeaveEvent struct {
	Range jRange
}

func (e jScopeLeaveEvent) Type() jEventType { return jEvScopeLeave }
func (e jScopeLeaveEvent) GetRange() jRange { return e.Range }

type jLocalDeclEvent struct {
	SourceID string
	Name     string
	DataType string
	Range    jRange
}

func (e jLocalDeclEvent) Type() jEventType { return jEvLocalDecl }
func (e jLocalDeclEvent) GetRange() jRange { return e.Range }

type jStartDeltaScopeEvent struct {
	SourceID   string
	MergeScope string
	Range      jRange
}

func (e jStartDeltaScopeEvent) Type() jEventType { return jEvStartDelta }
func (e jStartDeltaScopeEvent) GetRange() jRange { return e.Range }

type jEndDeltaScopeEvent struct {
	SourceID   string
	MergeScope string
	Range      jRange
}

func (e jEndDeltaScopeEvent) Type() jEventType { return jEvEndDelta }
func (e jEndDeltaScopeEvent) GetRange() jRange { return e.Range }

type jParseErrorEvent struct {
	SourceID string
	Message  string
	Range    jRange
}

func (e jParseErrorEvent) Type() jEventType { return jEvParseError }
func (e jParseErrorEvent) GetRange() jRange { return e.Range }

func jAnnoNames(list []*jAnno) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.Name)
	}
	return out
}

type jStaging struct {
	sourceID string
	events   []jEvent
	started  bool
	done     bool
	broken   bool
}

func newJStaging(sourceID string) *jStaging {
	return &jStaging{sourceID: sourceID, events: make([]jEvent, 0, 64)}
}

func (st *jStaging) Emit(evt jEvent) error {
	if st == nil || st.done {
		return nil
	}
	switch evt.Type() {
	case jEvStartFile:
		st.started = true
		st.events = st.events[:0]
		st.broken = false
	case jEvEndFile:
		st.done = true
	case jEvParseError:
		st.broken = true
		st.done = true
	}
	st.events = append(st.events, evt)
	return nil
}

func (st *jStaging) partial() bool { return st != nil && st.broken }

func (st *jStaging) commit() *jSourceFacts {
	if st == nil || !st.started {
		return nil
	}
	col := &jCollector{sourceID: st.sourceID}
	for _, evt := range st.events {
		col.consume(evt)
	}
	col.partial = st.broken
	return col.facts()
}

type jCollector struct {
	sourceID string
	pkg      string
	imports  []string
	types    []jKindFact
	methods  []jMethodFact
	fields   []jFieldFact
	partial  bool
}

type jKindFact struct {
	Kind        jKind
	Name        string
	Modifiers   []string
	Annotations []string
	Range       jRange
	Doc         string
}

type jMethodFact struct {
	Name        string
	ReturnType  string
	IsCtor      bool
	Modifiers   []string
	Annotations []string
	Parameters  []*jParam
	Exceptions  []string
	Range       jRange
	Doc         string
}

type jFieldFact struct {
	Name        string
	DataType    string
	IsEnumConst bool
	Modifiers   []string
	Annotations []string
	Range       jRange
	Doc         string
}

func (c *jCollector) consume(evt jEvent) {
	switch e := evt.(type) {
	case jPackageDeclEvent:
		c.pkg = e.Name
	case jImportDeclEvent:
		c.imports = append(c.imports, e.Name)
	case jTypeDeclEvent:
		c.types = append(c.types, jKindFact{
			Kind: e.Kind, Name: e.Name, Modifiers: e.Modifiers,
			Annotations: e.Annotations, Range: e.Range, Doc: e.Doc,
		})
	case jFieldDeclEvent:
		c.fields = append(c.fields, jFieldFact{
			Name: e.Name, DataType: e.DataType, IsEnumConst: e.IsEnumConst,
			Modifiers: e.Modifiers, Annotations: e.Annotations, Range: e.Range, Doc: e.Doc,
		})
	case jMethodDeclEvent:
		c.methods = append(c.methods, jMethodFact{
			Name: e.Name, ReturnType: e.ReturnType, Modifiers: e.Modifiers,
			Annotations: e.Annotations, Parameters: e.Parameters,
			Exceptions: e.Exceptions, Range: e.Range, Doc: e.Doc,
		})
	case jCtorDeclEvent:
		c.methods = append(c.methods, jMethodFact{
			Name: e.Name, ReturnType: e.Name, IsCtor: true, Modifiers: e.Modifiers,
			Annotations: e.Annotations, Parameters: e.Parameters,
			Exceptions: e.Exceptions, Range: e.Range, Doc: e.Doc,
		})
	}
}

type jSourceFacts struct {
	Package string
	Imports []string
	Types   []jKindFact
	Methods []jMethodFact
	Fields  []jFieldFact
	Partial bool
}

func (c *jCollector) facts() *jSourceFacts {
	return &jSourceFacts{
		Package: c.pkg,
		Imports: c.imports,
		Types:   c.types,
		Methods: c.methods,
		Fields:  c.fields,
		Partial: c.partial,
	}
}
