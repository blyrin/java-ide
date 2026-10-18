package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type treeNode struct {
	name string
	path string
	dir  bool
}

type editorTab struct {
	path     string
	name     string
	ed       *editor.State
	readOnly bool
}

type sourceLoc struct {
	file string
	line int
	col  int
}

type consoleKind int

const (
	consoleOut consoleKind = iota
	consoleErr
	consoleInfo
	consoleInput
)

type consoleEntry struct {
	kind consoleKind
	text string
	at   time.Time
	loc  *sourceLoc
}

type session struct {
	app         *app
	name        string
	path        string
	mainClass   string
	javaCommand string
	runs        []runConfig
	activeRun   int
	buildTool   string
	encoding    string
	settings    appSettings
	mono        string
	engine      *engine
	root        []*treeNode
	open        map[string]*bool
	showHidden  bool

	tabs   []*editorTab
	active int
	ed     *editor.State

	dbg         *debugger
	bpMu        sync.Mutex
	breakpoints []debugBreakpoint

	currentPath      string
	console          []consoleEntry
	consoleList      ui.ListState
	consoleSel       int
	consoleInput     string
	running          bool
	exitCode         int
	sideTab          int
	debugTab         int
	debugVars        ui.OutlineState[string]
	debugFramesList  ui.ListState
	debugWatchesList ui.ListState
	debugThreadsList ui.ListState
	debugBreaksList  ui.ListState
	structure        []*javaMember
	structureVersion uint64
	outline          ui.OutlineState[*javaMember]
	find             findState
	buildErrors      []buildError
	todos            []buildError
	problemsList     ui.ListState
	problemsSel      int
	git              gitState
	symbols          *symbolIndex
	recent           []string
	notice           string
	noticeKind       string
	watchStop        func()
	pending          []func()

	treeSplit    float32
	consoleSplit float32
}

func newSession(a *app, name, path, mono string) *session {
	s := &session{
		app:          a,
		name:         name,
		path:         path,
		mono:         mono,
		settings:     a.cfg.Settings.normalized(),
		engine:       &engine{},
		open:         map[string]*bool{},
		showHidden:   a.cfg.Settings.ShowHidden,
		active:       -1,
		console:      []consoleEntry{{kind: consoleInfo, text: "控制台中尚未启动任何进程", at: time.Now()}},
		consoleList:  ui.ListState{FollowEnd: a.cfg.settings().ConsoleFollow},
		consoleSel:   -1,
		problemsSel:  -1,
		find:         findState{sel: -1, opened: -1},
		exitCode:     -1,
		treeSplit:    220,
		consoleSplit: 420,
	}
	cfg := loadProjectConfigure(path)
	if _, err := os.Stat(projectConfigureFile(path)); err != nil {
		writeProjectConfigure(path, projectConfigure{})
	}
	s.mainClass, s.javaCommand = cfg.MainClass, cfg.JavaCommand
	s.runs, s.activeRun, s.buildTool, s.encoding = cfg.RunConfigs, cfg.ActiveRun, cfg.BuildTool, cfg.Encoding
	if len(s.runs) == 0 {
		s.runs = []runConfig{{Name: "当前配置", MainClass: cfg.MainClass}}
		s.activeRun = 0
	}
	s.breakpoints = cfg.Breakpoints
	s.root = treeLevel(path, s.showHidden)
	return s
}

func (s *session) close() {
	s.saveAll()
	s.cancelSearch()
	if s.dbg != nil {
		s.dbg.stop()
	}
	s.engine.stop()
	if s.watchStop != nil {
		s.watchStop()
		s.watchStop = nil
	}
}

func (s *session) later(fn func()) { s.pending = append(s.pending, fn) }

func (s *session) runPending() {
	if len(s.pending) == 0 {
		return
	}
	actions := s.pending
	s.pending = nil
	for _, fn := range actions {
		fn()
	}
}

func (s *session) openFlag(path string) *bool {
	if b, ok := s.open[path]; ok {
		return b
	}
	b := new(bool)
	s.open[path] = b
	return b
}

func (s *session) refreshTree() { s.root = treeLevel(s.path, s.showHidden) }

func (s *session) activeRunConfig() runConfig {
	if s.activeRun >= 0 && s.activeRun < len(s.runs) {
		return s.runs[s.activeRun]
	}
	return runConfig{Name: "当前配置", MainClass: s.mainClass}
}

func (s *session) setRunConfig(rc runConfig) {
	if s.activeRun < 0 || s.activeRun >= len(s.runs) {
		s.runs = append(s.runs, rc)
		s.activeRun = len(s.runs) - 1
		return
	}
	s.runs[s.activeRun] = rc
	if rc.MainClass != "" {
		s.mainClass = rc.MainClass
	}
	s.saveConfigure()
}

func (s *session) setActiveRunConfig(i int) {
	if i < 0 || i >= len(s.runs) {
		return
	}
	s.activeRun = i
	if rc := s.runs[i]; rc.MainClass != "" {
		s.mainClass = rc.MainClass
	}
	s.saveConfigure()
}

func (s *session) addRunConfig() {
	s.runs = append(s.runs, runConfig{Name: "新建配置", MainClass: s.mainClass})
	s.activeRun = len(s.runs) - 1
	s.saveConfigure()
}

func (s *session) removeRunConfig() {
	if len(s.runs) <= 1 || s.activeRun < 0 || s.activeRun >= len(s.runs) {
		return
	}
	s.runs = append(s.runs[:s.activeRun], s.runs[s.activeRun+1:]...)
	s.activeRun = min(s.activeRun, len(s.runs)-1)
	if rc := s.activeRunConfig(); rc.MainClass != "" {
		s.mainClass = rc.MainClass
	}
	s.saveConfigure()
}

func (s *session) runConfigNames() []string {
	out := make([]string, len(s.runs))
	for i, r := range s.runs {
		out[i] = r.Name
	}
	return out
}

func (s *session) saveConfigure() {
	writeProjectConfigure(s.path, projectConfigure{
		MainClass: s.mainClass, JavaCommand: s.javaCommand,
		RunConfigs: s.runs, ActiveRun: s.activeRun, BuildTool: s.buildTool, Encoding: s.encoding,
		Breakpoints: s.breakpoints,
	})
}

func (s *session) applySettings(st appSettings) {
	s.settings, s.mono, s.showHidden = st, st.monoFamily(), st.ShowHidden
	s.refreshTree()
	for _, tb := range s.tabs {
		s.applyEditorOptions(tb)
	}
}

func (s *session) applyEditorOptions(tb *editorTab) {
	if tb == nil || tb.ed == nil {
		return
	}
	st := s.settings.normalized()
	tb.ed.Font = ui.Font{Family: s.mono, Size: st.FontSize}
	tb.ed.TabSize = st.TabSize
	tb.ed.SoftWrap = st.SoftWrap
	tb.ed.LineHeight = 1.55
	tb.ed.WrapGuides = s.wrapGuides(st)
	if st.LineNumbers {
		tb.ed.LineNumbers = editor.LineNumbersOn
	} else {
		tb.ed.LineNumbers = editor.LineNumbersOff
	}
}

func (s *session) appendConsole(text string) { s.appendOutput(text, consoleOut) }

func (s *session) appendOutput(text string, kind consoleKind) {
	at := time.Now()
	for _, line := range consoleLines(text) {
		s.console = append(s.console, consoleEntry{kind: kind, text: line, at: at})
	}
	if len(s.console) > 5000 {
		s.console = s.console[len(s.console)-5000:]
	}
}

func (s *session) appendError(text string) {
	for _, line := range consoleLines(text) {
		loc := parseErrorLocation(line)
		if loc != nil {
			loc.file = parseSourcePath(s.path, loc.file)
		}
		s.console = append(s.console, consoleEntry{kind: consoleErr, text: line, at: time.Now(), loc: loc})
	}
	if len(s.console) > 5000 {
		s.console = s.console[len(s.console)-5000:]
	}
}

func (s *session) clearConsole() {
	s.console = nil
	s.consoleSel = -1
}

func (s *session) status() string {
	if s.running {
		return "运行中"
	}
	if s.exitCode < 0 {
		return "未运行"
	}
	return "已停止"
}

func (s *session) activeTab() *editorTab {
	if s.active >= 0 && s.active < len(s.tabs) {
		return s.tabs[s.active]
	}
	return nil
}

func (s *session) newEditorState() *editor.State {
	st := s.settings.normalized()
	ed := &editor.State{
		Label:       "代码编辑器",
		Placeholder: "从左侧文件树选择一个文件",
		Font:        ui.Font{Family: s.mono, Size: st.FontSize},
		TabSize:     st.TabSize,
		Indent:      editor.IndentSpaces,
		LineHeight:  1.55,
		SoftWrap:    st.SoftWrap,
		WrapGuides:  s.wrapGuides(st),
		LineNumbers: editor.LineNumbersOn,
	}
	if !st.LineNumbers {
		ed.LineNumbers = editor.LineNumbersOff
	}
	ed.OnCompletion = func(req editor.CompletionRequest) { s.onCompletion(req) }
	ed.OnHover = func(req editor.HoverRequest) { s.onHover(req) }
	ed.OnDefinition = func(offset int) { s.onDefinition(offset) }
	ed.ContextMenu = s.editorMenu
	return ed
}

func (s *session) wrapGuides(st appSettings) []int {
	if !st.ColumnGuide {
		return nil
	}
	return []int{80, 100}
}

func (s *session) editorMenu(m *ui.Menu) {
	if s.ed == nil {
		return
	}
	word := s.wordUnderCaret()
	if word != "" {
		if m.Item("跳转到定义").Shortcut(ui.Ctrl, ui.KeyB).Chosen() {
			s.onDefinition(s.ed.Cursor())
		}
		if m.Item("查找 " + word + " 的引用").Chosen() {
			s.sideTab = sideFind
			s.find.query = word
			s.find.searched = false
			s.find.focus = findFocusQuery
			s.runSearch()
		}
		m.Separator()
	}
	if m.Item("切换断点").Chosen() {
		if s.ed != nil {
			s.toggleBreakpoint(s.currentPath, s.ed.CursorPos().Line+1)
		}
	}
	m.Separator()
	if m.Item("注释/取消注释").Shortcut(ui.Ctrl, ui.KeySlash).Chosen() {
		s.ed.Run(editor.ToggleComment)
	}
	if m.Item("复制当前行").Chosen() {
		s.ed.Run(editor.DuplicateLinesDown)
	}
	if m.Item("删除当前行").Chosen() {
		s.ed.Run(editor.DeleteLine)
	}
	if m.Item("转为大写").Chosen() {
		s.ed.Run(editor.Uppercase)
	}
	if m.Item("转为小写").Chosen() {
		s.ed.Run(editor.Lowercase)
	}
	m.Separator()
	if m.Item("折叠全部").Chosen() {
		s.ed.Run(editor.FoldAll)
	}
	if m.Item("展开全部").Chosen() {
		s.ed.Run(editor.UnfoldAll)
	}
}

func (s *session) wordUnderCaret() string {
	if s.ed == nil || s.ed.Buffer == nil {
		return ""
	}
	word, _, _ := wordAt(s.editorText(), s.ed.Cursor())
	return word
}

func (s *session) setEditor(text, name string, readOnly bool) {
	tb := s.activeTab()
	if tb == nil || tb.path != s.currentPath {
		tb = s.openTab(s.currentPath, name, readOnly)
	}
	if tb.ed == nil {
		tb.ed = s.newEditorState()
		s.ed = tb.ed
	}
	if tb.ed.OnGutterClick == nil {
		path := tb.path
		tb.ed.OnGutterClick = func(line int) { s.toggleBreakpoint(path, line+1) }
	}
	if tb.ed.Buffer == nil {
		tb.ed.Buffer = editor.NewBuffer(text)
	} else {
		tb.ed.Buffer.Reset(text)
	}
	tb.ed.Buffer.SetLanguage(editor.LanguageFor(name))
	tb.ed.ReadOnly = readOnly
	tb.readOnly = readOnly
	tb.name = name
	s.applyEditorOptions(tb)
	s.structure, s.structureVersion = nil, 0
	s.refreshProblems()
}

func (s *session) openTab(path, name string, readOnly bool) *editorTab {
	for i, tb := range s.tabs {
		if tb.path == path {
			s.active = i
			s.ed = tb.ed
			s.currentPath = path
			return tb
		}
	}
	tb := &editorTab{path: path, name: name, readOnly: readOnly}
	s.tabs = append(s.tabs, tb)
	s.active = len(s.tabs) - 1
	s.currentPath = path
	return tb
}

func (s *session) rememberFile(path string) {
	s.recent = append([]string{path}, slices.DeleteFunc(s.recent, func(p string) bool { return p == path })...)
	if len(s.recent) > 20 {
		s.recent = s.recent[:20]
	}
}

func (s *session) findTab(path string) *editorTab {
	for _, tb := range s.tabs {
		if tb.path == path {
			return tb
		}
	}
	return nil
}

func (s *session) activateTab(path string) {
	for i, tb := range s.tabs {
		if tb.path == path {
			s.active, s.ed, s.currentPath = i, tb.ed, tb.path
			s.structure, s.structureVersion = nil, 0
			return
		}
	}
}

func (s *session) switchTab(i int) {
	if i < 0 || i >= len(s.tabs) || i == s.active {
		return
	}
	s.saveCurrentFile()
	s.active = i
	s.ed = s.tabs[i].ed
	s.currentPath = s.tabs[i].path
	s.structure, s.structureVersion = nil, 0
}

func (s *session) saveAll() {
	for _, tb := range s.tabs {
		if tb == nil || tb.readOnly || tb.ed == nil || tb.ed.Buffer == nil || !tb.ed.Buffer.Modified() {
			continue
		}
		os.WriteFile(tb.path, []byte(tb.ed.Buffer.String()), 0o644)
		tb.ed.Buffer.MarkSaved()
	}
}

func (s *session) classpath() string {
	switch s.buildToolName() {
	case buildMaven:
		dir := filepath.Join(s.path, "target", "classes")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	case buildGradle:
		dir := filepath.Join(s.path, "build", "classes", "java", "main")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return filepath.Join(s.path, "out")
}

func (s *session) closeTab(i int) {
	if i < 0 || i >= len(s.tabs) {
		return
	}
	s.tabs = append(s.tabs[:i], s.tabs[i+1:]...)
	if len(s.tabs) == 0 {
		s.active = -1
		s.ed = nil
		s.currentPath = ""
		return
	}
	if s.active >= len(s.tabs) {
		s.active = len(s.tabs) - 1
	}
	s.ed = s.tabs[s.active].ed
	s.currentPath = s.tabs[s.active].path
}

func (s *session) editorText() string {
	if s.ed == nil || s.ed.Buffer == nil {
		return ""
	}
	return s.ed.Buffer.String()
}

func (s *session) saveCurrentFile() error {
	tb := s.activeTab()
	if tb == nil || tb.readOnly || tb.ed == nil || tb.path == "" {
		return nil
	}
	if err := os.WriteFile(tb.path, []byte(tb.ed.Buffer.String()), 0o644); err != nil {
		return err
	}
	tb.ed.Buffer.MarkSaved()
	s.refreshProblems()
	return nil
}

func treeLevel(dir string, showHidden bool) []*treeNode {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	nodes := make([]*treeNode, 0, len(entries))
	for _, e := range entries {
		if !showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		nodes = append(nodes, &treeNode{name: e.Name(), path: filepath.Join(dir, e.Name()), dir: e.IsDir()})
	}
	slices.SortStableFunc(nodes, func(a, b *treeNode) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(a.name, b.name)
	})
	return nodes
}
