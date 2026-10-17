package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

type debugState int

const (
	debugIdle debugState = iota
	debugLaunching
	debugRunning
	debugSuspended
)

const (
	maxDebugChildren = 200
)

type debugBreakpoint struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Enabled   bool   `json:"enabled"`
	Condition string `json:"condition,omitempty"`
}

type debugFrame struct {
	id    int64
	name  string
	class string
	file  string
	line  int
}

type debugThread struct {
	id     int64
	name   string
	status int
}

type debugVar struct {
	key        string
	name       string
	typeName   string
	value      string
	tag        byte
	objID      int64
	numi       int64
	numf       float64
	array      bool
	expandable bool
	children   []*debugVar
	loaded     bool
	loading    bool
}

type debugClassInfo struct {
	id        int64
	signature string
	methods   map[int64]string
	lines     map[int64][][2]int64
}

type debugger struct {
	s         *session
	conn      *jdwpConn
	jobs      chan func()
	jobOne    sync.Once
	beginOnce sync.Once

	mu         sync.Mutex
	state      debugState
	info       map[int64]*debugClassInfo
	typeSigs   map[int64]string
	sigToType  map[string]int64
	sigForFile map[string]string
	classReq   map[string]int
	bpReq      map[string][]int
	tempReq    map[string]bool
	reqKey     map[int]string
	hit        map[string]bool
	fileFor    map[string]string

	thread      int64
	threads     []debugThread
	frames      []debugFrame
	frame       int
	vars        []*debugVar
	varByKey    map[string]*debugVar
	watches     []string
	watchVals   []string
	currentFile string
	currentLine int
	sources     []string
}

func (s *session) breakpointAt(file string, line int) (debugBreakpoint, bool) {
	for _, bp := range s.breakpointsSnapshot() {
		if bp.Line == line && filepath.Clean(bp.File) == filepath.Clean(file) {
			return bp, true
		}
	}
	return debugBreakpoint{}, false
}

func (s *session) breakpointsSnapshot() []debugBreakpoint {
	s.bpMu.Lock()
	defer s.bpMu.Unlock()
	return append([]debugBreakpoint(nil), s.breakpoints...)
}

func (s *session) withBreakpoints(fn func([]debugBreakpoint) []debugBreakpoint) {
	s.bpMu.Lock()
	s.breakpoints = fn(s.breakpoints)
	s.bpMu.Unlock()
	s.saveConfigure()
	s.dbgSyncBreakpoints()
}

func (s *session) toggleBreakpoint(file string, line int) {
	if line <= 0 || file == "" {
		return
	}
	if _, ok := s.breakpointAt(file, line); ok {
		s.removeBreakpointAt(file, line)
		return
	}
	s.withBreakpoints(func(bps []debugBreakpoint) []debugBreakpoint {
		out := append(append([]debugBreakpoint(nil), bps...), debugBreakpoint{File: file, Line: line, Enabled: true})
		sortBreakpointList(out)
		return out
	})
}

func (s *session) removeBreakpointAt(file string, line int) {
	s.withBreakpoints(func(bps []debugBreakpoint) []debugBreakpoint {
		out := make([]debugBreakpoint, 0, len(bps))
		for _, e := range bps {
			if e.Line == line && filepath.Clean(e.File) == filepath.Clean(file) {
				continue
			}
			out = append(out, e)
		}
		return out
	})
}

func (s *session) removeBreakpoint(bp debugBreakpoint) {
	s.removeBreakpointAt(bp.File, bp.Line)
}

func (s *session) setBreakpointEnabled(file string, line int, on bool) {
	s.withBreakpoints(func(bps []debugBreakpoint) []debugBreakpoint {
		out := append([]debugBreakpoint(nil), bps...)
		for i := range out {
			if out[i].Line == line && filepath.Clean(out[i].File) == filepath.Clean(file) {
				out[i].Enabled = on
				break
			}
		}
		return out
	})
}

func (s *session) setBreakpointCondition(file string, line int, cond string) {
	s.withBreakpoints(func(bps []debugBreakpoint) []debugBreakpoint {
		out := append([]debugBreakpoint(nil), bps...)
		for i := range out {
			if out[i].Line == line && filepath.Clean(out[i].File) == filepath.Clean(file) {
				out[i].Condition = cond
				break
			}
		}
		return out
	})
}

func (s *session) clearBreakpoints() {
	if len(s.breakpointsSnapshot()) == 0 {
		return
	}
	s.withBreakpoints(func([]debugBreakpoint) []debugBreakpoint { return nil })
}

func (s *session) enableAllBreakpoints(on bool) {
	s.withBreakpoints(func(bps []debugBreakpoint) []debugBreakpoint {
		out := append([]debugBreakpoint(nil), bps...)
		for i := range out {
			out[i].Enabled = on
		}
		return out
	})
}

func sortBreakpointList(bps []debugBreakpoint) {
	sort.SliceStable(bps, func(i, j int) bool {
		if bps[i].File != bps[j].File {
			return bps[i].File < bps[j].File
		}
		return bps[i].Line < bps[j].Line
	})
}

func (s *session) revealDebugLocation(file string, line int) {
	if file == "" {
		return
	}
	if tb := s.activeTab(); tb == nil || filepath.Clean(tb.path) != filepath.Clean(file) {
		s.app.openFileAt(file, line, 0)
		return
	}
	if s.ed != nil && line > 0 {
		s.ed.GoTo(editor.Pos{Line: max(line-1, 0), Col: 0})
	}
}

func (s *session) dbgSyncBreakpoints() {
	d := s.dbg
	if d == nil {
		return
	}
	d.post(func() { d.syncBreakpoints() })
}

func (a *app) toggleBreakpointAtCaret() {
	s := a.sess
	if s == nil || s.ed == nil {
		return
	}
	tb := s.activeTab()
	if tb == nil {
		return
	}
	s.toggleBreakpoint(tb.path, s.ed.CursorPos().Line+1)
}

func (a *app) editBreakpointConditionAtCaret() {
	s := a.sess
	if s == nil || s.ed == nil {
		return
	}
	tb := s.activeTab()
	if tb == nil {
		return
	}
	line := s.ed.CursorPos().Line + 1
	if bp, ok := s.breakpointAt(tb.path, line); ok {
		a.openConditionDialog(bp)
		return
	}
	a.showError("当前行没有断点，请先在行号槽点击设置断点")
}

func freeDebugPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func jdwpAgentArgs(port int) string {
	return fmt.Sprintf("-agentlib:jdwp=transport=dt_socket,server=y,suspend=y,address=127.0.0.1:%d,quiet=y", port)
}

func (s *session) startDebug(main string, rc runConfig, st appSettings, sources []string) {
	if s.dbg != nil || main == "" {
		return
	}
	port, err := freeDebugPort()
	if err != nil {
		s.appendError("无法分配调试端口：" + err.Error())
		return
	}
	d := &debugger{
		s:          s,
		jobs:       make(chan func(), 64),
		state:      debugLaunching,
		info:       map[int64]*debugClassInfo{},
		typeSigs:   map[int64]string{},
		sigToType:  map[string]int64{},
		sigForFile: map[string]string{},
		classReq:   map[string]int{},
		bpReq:      map[string][]int{},
		tempReq:    map[string]bool{},
		reqKey:     map[int]string{},
		hit:        map[string]bool{},
		fileFor:    map[string]string{},
		varByKey:   map[string]*debugVar{},
		sources:    sources,
	}
	s.dbg = d
	s.running = true
	s.exitCode = -1
	s.appendOutput("正在以调试模式启动 "+main+"（调试端口 "+strconv.Itoa(port)+"）\n", consoleInfo)
	go func() {
		res := s.engine.run(s.path, runOptions{
			javaCommand: s.javaCommand, mainClass: main, classpath: s.classpath(),
			vmArgs: rc.vmArguments(), programArgs: rc.programArguments(),
			env: rc.environment(), workDir: rc.WorkDir, jdkHome: st.JdkHome,
			debugPort: port,
		}, func(chunk string) {
			s.app.update(func() { s.appendOutput(chunk, consoleOut) })
		})
		d.reportUnhit()
		s.app.update(func() {
			if s.dbg == d {
				s.dbg = nil
			}
			s.running = false
			s.exitCode = res.ExitCode
			s.appendOutput(fmt.Sprintf("\n调试会话已结束，退出代码 %d\n", res.ExitCode), consoleInfo)
		})
	}()
	go d.connect(port)
}

func (d *debugger) connect(port int) {
	s := d.s
	conn, err := dialJDWP(port, 40*time.Second, d.onEvents)
	if err != nil {
		s.app.update(func() {
			if s.dbg == d {
				s.dbg = nil
				s.running = false
			}
			s.appendError("调试连接失败：" + err.Error())
		})
		return
	}
	d.mu.Lock()
	d.conn = conn
	d.mu.Unlock()
	d.startJobs()
	if err := conn.idSizes(); err != nil {
		s.app.update(func() { s.appendError("调试初始化失败：" + err.Error()) })
		return
	}
	ver, _ := conn.version()
	s.app.update(func() { s.appendOutput("调试器已连接："+ver+"\n", consoleInfo) })
	d.watchReady()
}

func (d *debugger) watchReady() {
	go func() {
		time.Sleep(20 * time.Second)
		if d.currentState() != debugLaunching {
			return
		}
		d.s.app.update(func() { d.s.appendError("调试目标未发送就绪事件，调试会话中止") })
		d.stop()
	}()
}

func (d *debugger) startJobs() {
	d.jobOne.Do(func() {
		go func() {
			for {
				select {
				case fn := <-d.jobs:
					fn()
				case <-d.conn.dead:
					return
				}
			}
		}()
	})
}

func (d *debugger) connection() *jdwpConn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn
}

func (d *debugger) post(fn func()) {
	conn := d.connection()
	if conn == nil {
		return
	}
	select {
	case d.jobs <- fn:
	case <-conn.dead:
	default:
		d.s.app.update(func() { d.s.appendOutput("调试命令队列已满，本次操作被忽略\n", consoleInfo) })
	}
}

func (d *debugger) publish(fn func()) {
	s := d.s
	s.app.update(func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		fn()
	})
}

func (d *debugger) setState(st debugState) {
	d.mu.Lock()
	d.state = st
	d.mu.Unlock()
}

func (d *debugger) setStopped(thread int64, frames []debugFrame) {
	d.mu.Lock()
	d.thread = thread
	d.frames = frames
	d.frame = 0
	if len(frames) > 0 {
		d.currentFile, d.currentLine = frames[0].file, frames[0].line
	}
	d.state = debugSuspended
	d.mu.Unlock()
}

func (d *debugger) setRunning() {
	d.mu.Lock()
	d.state = debugRunning
	d.vars = nil
	d.varByKey = map[string]*debugVar{}
	d.frames = nil
	d.frame = 0
	d.currentFile, d.currentLine = "", 0
	d.mu.Unlock()
}

func (d *debugger) keyOfRequest(id int) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reqKey[id]
}

func (d *debugger) reportUnhit() {
	missed := make([]string, 0, 4)
	for _, bp := range d.s.breakpointsSnapshot() {
		if !bp.Enabled {
			continue
		}
		key := bpKey(bp.File, bp.Line)
		d.mu.Lock()
		hit, installed := d.hit[key], len(d.bpReq[key]) > 0
		d.mu.Unlock()
		if hit {
			continue
		}
		rel := relativeTo(d.s.path, bp.File) + ":" + strconv.Itoa(bp.Line)
		if installed {
			missed = append(missed, rel)
		} else {
			missed = append(missed, rel+"（未生效）")
		}
	}
	if len(missed) == 0 {
		return
	}
	d.log("未命中断点：" + strings.Join(missed, "、") + "（所在行可能没有可执行代码，或对应代码没有执行）")
}

func (d *debugger) log(text string) {
	s := d.s
	s.app.update(func() { s.appendOutput(text+"\n", consoleInfo) })
}

func (d *debugger) stop() {
	s := d.s
	d.reportUnhit()
	if conn := d.connection(); conn != nil {
		go func() { conn.close() }()
	}
	s.engine.stop()
	s.app.update(func() {
		if s.dbg == d {
			s.dbg = nil
		}
		s.running = false
		s.exitCode = -1
		s.appendOutput("调试已停止\n", consoleInfo)
	})
}

func (d *debugger) onEvents(batch jdwpEventBatch) {
	prepared, stopped := false, false
	for _, e := range batch.events {
		switch e.kind {
		case evVMStart:
			d.onVMStart()
		case evClassPrep:
			d.onClassPrepare(e)
			prepared = true
		case evBreakpoint:
			d.onStop(e, "断点")
			stopped = true
		case evSingleStep:
			d.onStop(e, "单步执行")
			stopped = true
		case evThreadStart, evThreadDeath:
			d.refreshThreads()
		case evVMDeath:
			d.mu.Lock()
			if d.s.dbg == d {
				d.state = debugIdle
			}
			d.mu.Unlock()
		}
	}
	if prepared && !stopped {
		d.resumeAfterPrepare()
	}
}

func (d *debugger) resumeAfterPrepare() {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	if state != debugRunning {
		return
	}
	if err := d.conn.resumeVM(); err != nil {
		return
	}
	d.setState(debugRunning)
}

func (d *debugger) onVMStart() {
	d.beginSession()
}

func (d *debugger) beginSession() {
	d.beginOnce.Do(func() {
		classes, err := d.conn.allClasses()
		if err != nil {
			classes = map[string]int64{}
		}
		d.mu.Lock()
		for sig, id := range classes {
			d.typeSigs[id] = sig
			d.sigToType[sig] = id
		}
		d.mu.Unlock()
		d.setState(debugRunning)
		d.log("调试会话已就绪")
		d.syncBreakpoints()
		if err := d.conn.resumeVM(); err != nil {
			d.s.app.update(func() { d.s.appendError("恢复执行失败：" + err.Error()) })
			return
		}
	})
}

func (d *debugger) onClassPrepare(e jdwpEvent) {
	d.mu.Lock()
	d.typeSigs[e.typeID] = e.signature
	d.sigToType[e.signature] = e.typeID
	d.mu.Unlock()
	d.resolveInClass(e.typeID, e.signature)
}

func (d *debugger) syncBreakpoints() {
	d.mu.Lock()
	registered := make(map[string]int, len(d.classReq))
	for k, v := range d.classReq {
		registered[k] = v
	}
	loaded := make(map[string]int64, len(d.sigToType))
	for sig, id := range d.sigToType {
		loaded[sig] = id
	}
	pending := make([]debugBreakpoint, 0, len(d.s.breakpointsSnapshot()))
	for _, bp := range d.s.breakpointsSnapshot() {
		if bp.Enabled {
			pending = append(pending, bp)
		}
	}
	d.mu.Unlock()

	names := map[string]bool{}
	for _, bp := range pending {
		names[dottedFromSignature(d.fileSignature(bp.File))] = true
	}
	for name := range names {
		if _, ok := registered[name]; ok {
			continue
		}
		id, err := d.conn.setEvent(evClassPrep, suspendAll, modClassMatch(name+"*"))
		if err != nil {
			continue
		}
		d.mu.Lock()
		d.classReq[name] = id
		d.mu.Unlock()
	}
	for sig, id := range loaded {
		d.resolveInClass(id, sig)
	}
}

func (d *debugger) fileSignature(file string) string {
	d.mu.Lock()
	if sig, ok := d.sigForFile[file]; ok {
		d.mu.Unlock()
		return sig
	}
	d.mu.Unlock()
	sig := classSignatureForFile(file)
	d.mu.Lock()
	d.sigForFile[file] = sig
	d.mu.Unlock()
	return sig
}

func (d *debugger) resolveInClass(typeID int64, sig string) {
	outer := sigOuter(sig)
	for _, bp := range d.s.breakpointsSnapshot() {
		if !bp.Enabled || d.fileSignature(bp.File) != outer {
			continue
		}
		key := bpKey(bp.File, bp.Line)
		d.mu.Lock()
		_, done := d.bpReq[key]
		temp := d.tempReq[key]
		d.mu.Unlock()
		if done || temp {
			continue
		}
		d.setBreakpointRequest(bp.File, bp.Line, typeID, key, false)
	}
}

func (d *debugger) setBreakpointRequest(file string, line int, typeID int64, key string, temp bool) {
	loc, actual, ok := d.locationOfLine(typeID, line)
	if !ok {
		if !temp && !d.hasBreakpointRequest(key) {
			d.log("断点 " + relativeTo(d.s.path, file) + ":" + strconv.Itoa(line) + " 未生效：该位置之后没有可执行代码")
		}
		return
	}
	id, err := d.conn.setEvent(evBreakpoint, suspendAll, modLocationOnly(d.conn.sizes, loc))
	if err != nil {
		return
	}
	d.mu.Lock()
	_, existed := d.bpReq[key]
	d.bpReq[key] = append(d.bpReq[key], id)
	d.reqKey[id] = key
	if temp {
		d.tempReq[key] = true
	}
	d.mu.Unlock()
	if temp {
		return
	}
	rel := relativeTo(d.s.path, file)
	if actual != line {
		if !existed {
			d.log("断点 " + rel + ":" + strconv.Itoa(line) + " 所在行没有可执行代码，已移到第 " + strconv.Itoa(actual) + " 行")
		}
		return
	}
	if !existed {
		d.log("已设置断点 " + rel + ":" + strconv.Itoa(line))
	}
}

func (d *debugger) hasBreakpointRequest(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.bpReq[key]) > 0
}

func (d *debugger) locationOfLine(typeID int64, line int) (jdwpLoc, int, bool) {
	methods, err := d.conn.methods(typeID)
	if err != nil {
		return jdwpLoc{}, 0, false
	}
	type hit struct {
		method int64
		index  int64
		source int
	}
	var exact, next *hit
	for _, m := range methods {
		_, _, lines, err := d.conn.lineTable(typeID, m.id)
		if err != nil {
			continue
		}
		for _, e := range lines {
			src := int(e[1])
			if src == line {
				if exact == nil || e[0] > exact.index {
					exact = &hit{method: m.id, index: e[0], source: src}
				}
				continue
			}
			if src < line {
				continue
			}
			if next == nil || src < next.source || (src == next.source && e[0] < next.index) {
				next = &hit{method: m.id, index: e[0], source: src}
			}
		}
	}
	pick := exact
	if pick == nil {
		pick = next
	}
	if pick == nil {
		return jdwpLoc{}, 0, false
	}
	return jdwpLoc{tag: tagObject, class: typeID, method: pick.method, index: pick.index}, pick.source, true
}

func (d *debugger) clearBreakpointRequests(key string) {
	d.mu.Lock()
	ids := d.bpReq[key]
	delete(d.bpReq, key)
	delete(d.tempReq, key)
	for _, id := range ids {
		delete(d.reqKey, id)
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.conn.clearEvent(evBreakpoint, id)
	}
}

func (d *debugger) onStop(e jdwpEvent, reason string) {
	thread := e.thread
	if key := d.keyOfRequest(e.requestID); key != "" {
		d.mu.Lock()
		d.hit[key] = true
		d.mu.Unlock()
	}
	frames, err := d.conn.frames(thread)
	if err != nil {
		d.s.app.update(func() { d.s.appendOutput("无法读取调用栈："+err.Error()+"\n", consoleErr) })
		return
	}
	if len(frames) == 0 {
		return
	}
	list := make([]debugFrame, 0, len(frames))
	for _, f := range frames {
		list = append(list, d.frameInfo(f))
	}
	key := bpKey(list[0].file, list[0].line)
	d.mu.Lock()
	temp := d.tempReq[key]
	d.mu.Unlock()
	if temp {
		d.clearBreakpointRequests(key)
	}
	if cond := d.conditionOf(list[0].file, list[0].line); cond != "" {
		res, evalErr := evalInThread(d, thread, 0, cond)
		if evalErr == nil && res != "true" {
			d.continueVM()
			return
		}
	}
	d.setStopped(thread, list)
	file, line := list[0].file, list[0].line
	d.s.app.update(func() { d.s.revealDebugLocation(file, line) })
	d.log("已在 " + relativeTo(d.s.path, list[0].file) + ":" + strconv.Itoa(list[0].line) + " 命中" + reason)
	d.loadVariables(thread, frames, 0)
	d.refreshThreads()
	d.evalWatches(thread)
}

func (d *debugger) conditionOf(file string, line int) string {
	bp, ok := d.s.breakpointAt(file, line)
	if !ok || !bp.Enabled {
		return ""
	}
	return strings.TrimSpace(bp.Condition)
}

func (d *debugger) frameInfo(f jdwpFrame) debugFrame {
	info := d.classInfo(f.loc.class)
	class := dottedFromSignature(info.signature)
	name := info.methods[f.loc.method]
	switch name {
	case "":
		name = "未知方法"
	case "<init>":
		name = simpleName(class)
	case "<clinit>":
		name = "静态初始化"
	}
	return debugFrame{
		id:    f.id,
		name:  name,
		class: class,
		file:  d.sourceFileOf(info.signature, f.loc.class),
		line:  d.lineOf(info, f.loc.method, f.loc.index),
	}
}

func (d *debugger) classInfo(id int64) *debugClassInfo {
	d.mu.Lock()
	if info, ok := d.info[id]; ok {
		d.mu.Unlock()
		return info
	}
	d.mu.Unlock()
	info := &debugClassInfo{id: id, methods: map[int64]string{}, lines: map[int64][][2]int64{}}
	if sig, err := d.conn.signature(id); err == nil {
		info.signature = sig
	}
	if ms, err := d.conn.methods(id); err == nil {
		for _, m := range ms {
			info.methods[m.id] = m.name
		}
	}
	d.mu.Lock()
	d.info[id] = info
	d.mu.Unlock()
	return info
}

func (d *debugger) lineOf(info *debugClassInfo, methodID, index int64) int {
	if lines, ok := info.lines[methodID]; ok {
		return lineFromTable(lines, index)
	}
	_, _, lines, err := d.conn.lineTable(info.id, methodID)
	if err != nil {
		info.lines[methodID] = nil
		return 0
	}
	info.lines[methodID] = lines
	return lineFromTable(lines, index)
}

func lineFromTable(lines [][2]int64, index int64) int {
	best := int64(-1)
	line := 0
	for _, e := range lines {
		if e[0] <= index && e[0] > best {
			best, line = e[0], int(e[1])
		}
	}
	if best < 0 && len(lines) > 0 {
		line = int(lines[0][1])
	}
	return line
}

func (d *debugger) sourceFileOf(sig string, typeID int64) string {
	d.mu.Lock()
	if f, ok := d.fileFor[sig]; ok {
		d.mu.Unlock()
		return f
	}
	d.mu.Unlock()
	name, err := d.conn.sourceFile(typeID)
	if err != nil {
		return ""
	}
	pkg := packageOfSignature(sig)
	candidate := filepath.Join(d.s.path, "src", filepath.FromSlash(pkg), name)
	if !fileExists(candidate) {
		candidate = ""
		for _, f := range d.sources {
			if strings.EqualFold(filepath.Base(f), name) {
				candidate = f
				break
			}
		}
	}
	d.mu.Lock()
	d.fileFor[sig] = candidate
	d.mu.Unlock()
	return candidate
}

func (d *debugger) loadVariables(thread int64, frames []jdwpFrame, index int) {
	if index < 0 || index >= len(frames) {
		return
	}
	vars := d.captureFrame(thread, frames[index])
	byKey := map[string]*debugVar{}
	indexVars(vars, byKey)
	d.publish(func() {
		d.vars = vars
		d.varByKey = byKey
	})
}

func indexVars(vars []*debugVar, into map[string]*debugVar) {
	for _, v := range vars {
		into[v.key] = v
		indexVars(v.children, into)
	}
}

func (d *debugger) captureFrame(thread int64, f jdwpFrame) []*debugVar {
	var out []*debugVar
	slots, err := d.conn.variableTable(f.loc.class, f.loc.method)
	if err != nil || len(slots) == 0 {
		out = append(out, &debugVar{key: "locals", name: "局部变量", value: "缺少调试信息（请使用 -g 参数编译）"})
	} else {
		nums := make([]int, 0, len(slots))
		tags := make([]byte, 0, len(slots))
		for _, s := range slots {
			nums = append(nums, s.slot)
			tags = append(tags, tagFromSig(s.sig))
		}
		if values, err := d.conn.localValues(thread, f.id, nums, tags); err == nil {
			for i, s := range slots {
				if i >= len(values) {
					break
				}
				out = append(out, d.newVar("locals/"+s.name, s.name, s.sig, values[i]))
			}
		}
	}
	if this, err := d.conn.thisObject(thread, f.id); err == nil && this.obj != 0 {
		node := d.newVar("this", "this", d.classSigOfType(this.obj), this)
		node.children = d.fieldsOf(this.obj, f.loc.class, "this/")
		node.expandable, node.loaded = len(node.children) > 0, len(node.children) > 0
		out = append(out, node)
	}
	if statics := d.staticFieldsOf(f.loc.class, "static/"); len(statics) > 0 {
		out = append(out, &debugVar{key: "static", name: "静态字段", expandable: true, loaded: true, children: statics})
	}
	return out
}

func (d *debugger) staticFieldsOf(typeID int64, prefix string) []*debugVar {
	fields, err := d.conn.fields(typeID)
	if err != nil {
		return nil
	}
	var ids []int64
	var kept []jdwpField
	for _, f := range fields {
		if !f.static {
			continue
		}
		ids = append(ids, f.id)
		kept = append(kept, f)
	}
	if len(ids) == 0 {
		return nil
	}
	values, err := d.conn.staticValues(typeID, ids)
	if err != nil {
		return nil
	}
	var out []*debugVar
	for i, f := range kept {
		if i >= len(values) {
			break
		}
		out = append(out, d.newVar(prefix+f.name, f.name, f.sig, values[i]))
	}
	return out
}

func (d *debugger) fieldsOf(objID, typeID int64, prefix string) []*debugVar {
	fields, err := d.conn.fields(typeID)
	if err != nil {
		return nil
	}
	var ids []int64
	var kept []jdwpField
	for _, f := range fields {
		if f.static {
			continue
		}
		ids = append(ids, f.id)
		kept = append(kept, f)
	}
	if len(ids) == 0 {
		return nil
	}
	values, err := d.conn.fieldValues(objID, ids)
	if err != nil {
		return nil
	}
	var out []*debugVar
	for i, f := range kept {
		if i >= len(values) {
			break
		}
		out = append(out, d.newVar(prefix+f.name, f.name, f.sig, values[i]))
	}
	return out
}

func (d *debugger) newVar(key, name, sig string, v jdwpValue) *debugVar {
	node := &debugVar{key: key, name: name, tag: v.tag, objID: v.obj, typeName: typeNameFromSig(sig)}
	switch {
	case v.tag == tagString:
		if v.obj == 0 {
			node.value = "null"
		} else if text, err := d.conn.stringValue(v.obj); err == nil {
			node.value = strconv.Quote(text)
		} else {
			node.value = "\"…\""
		}
	case v.isObject():
		switch {
		case v.obj == 0:
			node.value = "null"
		case v.tag == tagArray:
			node.array = true
			node.value = d.arrayLabel(v.obj)
			node.expandable = true
		default:
			node.value = d.objectLabel(v.obj)
			node.expandable = true
		}
	default:
		node.value = v.primitive()
		node.numi, node.numf = v.i, v.f
	}
	return node
}

func (d *debugger) arrayLabel(objID int64) string {
	n := d.conn.arrayLength(objID)
	if n < 0 {
		return "数组"
	}
	return fmt.Sprintf("%s[%d]", d.typeNameOfObject(objID), n)
}

func (d *debugger) objectLabel(objID int64) string {
	name := d.typeNameOfObject(objID)
	if name == "" {
		name = "对象"
	}
	return simpleName(name) + "@" + strconv.FormatInt(objID&0xfffffff, 16)
}

func (d *debugger) typeNameOfObject(objID int64) string {
	typeID := d.conn.objectType(objID)
	if typeID == 0 {
		return ""
	}
	return dottedFromSignature(d.signatureOf(typeID))
}

func (d *debugger) classSigOfType(objID int64) string {
	typeID := d.conn.objectType(objID)
	if typeID == 0 {
		return ""
	}
	return d.signatureOf(typeID)
}

func (d *debugger) signatureOf(typeID int64) string {
	d.mu.Lock()
	if sig, ok := d.typeSigs[typeID]; ok {
		d.mu.Unlock()
		return sig
	}
	d.mu.Unlock()
	sig, err := d.conn.signature(typeID)
	if err != nil {
		return ""
	}
	d.mu.Lock()
	d.typeSigs[typeID] = sig
	d.sigToType[sig] = typeID
	d.mu.Unlock()
	return sig
}

func (d *debugger) childrenOf(v *debugVar) []*debugVar {
	if v.tag == tagArray {
		n := d.conn.arrayLength(v.objID)
		if n <= 0 {
			return nil
		}
		count := min(n, maxDebugChildren)
		elemSig := elemSigOfType(d.typeNameOfObject(v.objID))
		values, err := d.conn.arrayValues(v.objID, 0, count, elemSig)
		if err != nil {
			return nil
		}
		var out []*debugVar
		for i, val := range values {
			out = append(out, d.newVar(v.key+"/"+strconv.Itoa(i), "["+strconv.Itoa(i)+"]", elemSig, val))
		}
		return out
	}
	if v.objID == 0 {
		return nil
	}
	typeID := d.conn.objectType(v.objID)
	if typeID == 0 {
		return nil
	}
	return d.fieldsOf(v.objID, typeID, v.key+"/")
}

func (d *debugger) expandVar(v *debugVar) {
	if v == nil || v.loaded || v.loading || !v.expandable || v.objID == 0 {
		return
	}
	v.loading = true
	go func() {
		kids := d.childrenOf(v)
		byKey := map[string]*debugVar{}
		indexVars(kids, byKey)
		d.publish(func() {
			v.children = kids
			v.loaded = true
			v.loading = false
			for k, n := range byKey {
				d.varByKey[k] = n
			}
		})
	}()
}

func (d *debugger) refreshThreads() {
	ids, err := d.conn.allThreads()
	if err != nil {
		return
	}
	list := make([]debugThread, 0, len(ids))
	for _, id := range ids {
		name, err := d.conn.threadName(id)
		if err != nil {
			continue
		}
		list = append(list, debugThread{id: id, name: name, status: d.conn.threadStatus(id)})
	}
	d.publish(func() { d.threads = list })
}

func (d *debugger) evalWatches(thread int64) {
	d.mu.Lock()
	exprs := append([]string(nil), d.watches...)
	index := d.frame
	d.mu.Unlock()
	if len(exprs) == 0 {
		d.publish(func() { d.watchVals = nil })
		return
	}
	vals := make([]string, len(exprs))
	for i, e := range exprs {
		v, err := evalInThread(d, thread, index, e)
		if err != nil {
			vals[i] = "错误：" + err.Error()
		} else {
			vals[i] = v
		}
	}
	d.publish(func() { d.watchVals = vals })
}

func (d *debugger) selectFrame(i int) {
	d.mu.Lock()
	if i < 0 || i >= len(d.frames) || i == d.frame || d.state != debugSuspended {
		d.mu.Unlock()
		return
	}
	d.frame = i
	file, line := d.frames[i].file, d.frames[i].line
	d.currentFile, d.currentLine = file, line
	d.mu.Unlock()
	if file != "" {
		d.s.later(func() { d.s.revealDebugLocation(file, line) })
	}
	d.post(func() {
		d.reloadVariables()
		d.evalWatches(d.currentThread())
	})
}

func (d *debugger) currentThread() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.thread
}

func (d *debugger) reloadVariables() {
	d.mu.Lock()
	thread, index := d.thread, d.frame
	d.mu.Unlock()
	frames, err := d.conn.frames(thread)
	if err != nil {
		return
	}
	d.loadVariables(thread, frames, index)
}

func (d *debugger) clearStoppedState() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.vars = nil
	d.varByKey = map[string]*debugVar{}
	d.frames = nil
	d.frame = 0
	d.currentFile, d.currentLine = "", 0
}

func (d *debugger) resume() {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	if state != debugSuspended {
		return
	}
	d.post(func() {
		if err := d.conn.resumeVM(); err != nil {
			d.s.app.update(func() { d.s.appendError("恢复执行失败：" + err.Error()) })
			return
		}
		d.setRunning()
	})
}

func (d *debugger) continueVM() {
	if err := d.conn.resumeVM(); err != nil {
		return
	}
	d.setRunning()
}

func (d *debugger) pause() {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	if state != debugRunning {
		return
	}
	d.post(func() {
		if err := d.conn.suspendVM(); err != nil {
			d.s.app.update(func() { d.s.appendError("暂停失败：" + err.Error()) })
			return
		}
		d.pickThreadAndLoad("已暂停")
	})
}

func (d *debugger) pickThreadAndLoad(reason string) {
	ids, err := d.conn.allThreads()
	if err != nil || len(ids) == 0 {
		return
	}
	thread := ids[0]
	for _, id := range ids {
		if name, err := d.conn.threadName(id); err == nil && name == "main" {
			thread = id
			break
		}
	}
	frames, err := d.conn.frames(thread)
	if err != nil || len(frames) == 0 {
		d.setState(debugSuspended)
		d.log("已暂停，当前没有可显示的调用栈")
		return
	}
	list := make([]debugFrame, 0, len(frames))
	for _, f := range frames {
		list = append(list, d.frameInfo(f))
	}
	d.setStopped(thread, list)
	d.log("线程 " + d.threadName(thread) + " " + reason)
	d.loadVariables(thread, frames, 0)
	d.refreshThreads()
}

func (d *debugger) threadName(id int64) string {
	name, err := d.conn.threadName(id)
	if err != nil {
		return "未知线程"
	}
	return name
}

func (d *debugger) step(depth int32) {
	d.mu.Lock()
	state, thread := d.state, d.thread
	d.mu.Unlock()
	if state != debugSuspended {
		return
	}
	d.post(func() {
		if _, err := d.conn.setEvent(evSingleStep, suspendThread, modStep(d.conn.sizes, thread, stepSizeLine, depth)); err != nil {
			d.s.app.update(func() { d.s.appendError("单步执行失败：" + err.Error()) })
			return
		}
		if err := d.conn.resumeThread(thread); err != nil {
			d.s.app.update(func() { d.s.appendError("恢复线程失败：" + err.Error()) })
			return
		}
		d.setRunning()
	})
}

func (d *debugger) runToCursor(file string, line int) {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	if state != debugSuspended || line <= 0 {
		return
	}
	d.post(func() {
		key := bpKey(file, line)
		if _, ok := d.s.breakpointAt(file, line); ok {
			d.s.app.update(func() { d.s.appendOutput("该行已有断点，直接继续执行\n", consoleInfo) })
			return
		}
		sig := d.fileSignature(file)
		d.mu.Lock()
		typeID, ok := d.sigToType[sig]
		d.mu.Unlock()
		if !ok {
			d.syncBreakpoints()
			d.mu.Lock()
			typeID, ok = d.sigToType[sig]
			d.mu.Unlock()
		}
		if !ok {
			d.s.app.update(func() { d.s.appendOutput("运行至光标失败：找不到对应的类\n", consoleInfo) })
			return
		}
		d.setBreakpointRequest(file, line, typeID, key, true)
		if err := d.conn.resumeVM(); err != nil {
			d.s.app.update(func() { d.s.appendError("恢复执行失败：" + err.Error()) })
			return
		}
		d.setRunning()
	})
}

func (d *debugger) selectThread(id int64) {
	d.mu.Lock()
	if d.state != debugSuspended || id == d.thread {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	d.post(func() {
		frames, err := d.conn.frames(id)
		if err != nil {
			d.s.app.update(func() { d.s.appendOutput("该线程未挂起，无法查看调用栈\n", consoleInfo) })
			return
		}
		list := make([]debugFrame, 0, len(frames))
		for _, f := range frames {
			list = append(list, d.frameInfo(f))
		}
		d.mu.Lock()
		d.thread, d.frames, d.frame = id, list, 0
		if len(list) > 0 {
			d.currentFile, d.currentLine = list[0].file, list[0].line
		}
		d.mu.Unlock()
		if len(list) > 0 {
			file, line := list[0].file, list[0].line
			d.s.app.update(func() { d.s.revealDebugLocation(file, line) })
		}
		d.loadVariables(id, frames, 0)
	})
}

func (d *debugger) addWatch(expr string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return
	}
	d.mu.Lock()
	d.watches = append(d.watches, expr)
	state, thread, index := d.state, d.thread, d.frame
	d.mu.Unlock()
	if state != debugSuspended {
		d.publish(func() { d.watchVals = append(d.watchVals, "程序未暂停") })
		return
	}
	d.post(func() {
		text, err := evalInThread(d, thread, index, expr)
		if err != nil {
			text = "错误：" + err.Error()
		}
		d.publish(func() { d.watchVals = append(d.watchVals, text) })
	})
}

func (d *debugger) removeWatch(i int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if i < 0 || i >= len(d.watches) {
		return
	}
	d.watches = append(d.watches[:i], d.watches[i+1:]...)
	if i < len(d.watchVals) {
		d.watchVals = append(d.watchVals[:i], d.watchVals[i+1:]...)
	}
}

func (d *debugger) evaluate(expr string, done func(string, error)) {
	d.mu.Lock()
	state, thread, index := d.state, d.thread, d.frame
	d.mu.Unlock()
	if state != debugSuspended {
		done("", fmt.Errorf("程序未暂停，无法计算表达式"))
		return
	}
	d.post(func() {
		text, err := evalInThread(d, thread, index, expr)
		d.s.app.update(func() { done(text, err) })
	})
}

func evalInThread(d *debugger, thread int64, index int, expr string) (string, error) {
	scope := &exprScope{d: d, thread: thread, index: index}
	return evalExpression(expr, scope)
}

func (d *debugger) currentState() debugState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

func (d *debugger) statusText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch d.state {
	case debugLaunching:
		return "调试启动中"
	case debugRunning:
		return "调试运行中"
	case debugSuspended:
		return "调试已暂停"
	}
	return "调试已结束"
}

func (d *debugger) current() (string, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.currentFile, d.currentLine
}

func (d *debugger) snapshot() (debugState, []debugFrame, int, []*debugVar, map[string]*debugVar, []debugThread, []string, []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state, d.frames, d.frame, d.vars, d.varByKey, d.threads, d.watches, d.watchVals
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func bpKey(file string, line int) string {
	return filepath.Clean(file) + "\x00" + strconv.Itoa(line)
}

func classSignatureForFile(file string) string {
	pkg := ""
	if b, err := os.ReadFile(file); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "package ") {
				pkg = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(t, "package ")), ";")
				break
			}
			if strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "class ") ||
				strings.HasPrefix(t, "public ") || strings.HasPrefix(t, "final ") {
				break
			}
		}
	}
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	if pkg == "" {
		return "L" + base + ";"
	}
	return "L" + strings.ReplaceAll(pkg, ".", "/") + "/" + base + ";"
}

func dottedFromSignature(sig string) string {
	s := strings.TrimPrefix(sig, "L")
	s = strings.TrimSuffix(s, ";")
	return strings.ReplaceAll(s, "/", ".")
}

func packageOfSignature(sig string) string {
	s := strings.TrimPrefix(sig, "L")
	s = strings.TrimSuffix(s, ";")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[:i]
	}
	return ""
}

func simpleName(dotted string) string {
	if i := strings.LastIndex(dotted, "."); i >= 0 {
		return dotted[i+1:]
	}
	return dotted
}

func sigOuter(sig string) string {
	s := strings.TrimPrefix(sig, "L")
	s = strings.TrimSuffix(s, ";")
	if i := strings.Index(s, "$"); i >= 0 {
		s = s[:i]
	}
	return "L" + s + ";"
}

func typeNameFromSig(sig string) string {
	switch sig {
	case "I":
		return "int"
	case "J":
		return "long"
	case "Z":
		return "boolean"
	case "C":
		return "char"
	case "B":
		return "byte"
	case "S":
		return "short"
	case "F":
		return "float"
	case "D":
		return "double"
	case "V":
		return "void"
	}
	if strings.HasPrefix(sig, "[") {
		return typeNameFromSig(sig[1:]) + "[]"
	}
	return dottedFromSignature(sig)
}

func elemSigOfType(typeName string) string {
	t := strings.TrimSuffix(typeName, "[]")
	switch t {
	case "int":
		return "I"
	case "long":
		return "J"
	case "boolean":
		return "Z"
	case "char":
		return "C"
	case "byte":
		return "B"
	case "short":
		return "S"
	case "float":
		return "F"
	case "double":
		return "D"
	}
	return "L" + strings.ReplaceAll(t, ".", "/") + ";"
}

func tagFromSig(sig string) byte {
	if sig == "" {
		return tagObject
	}
	switch sig[0] {
	case 'Z':
		return tagBoolean
	case 'B':
		return tagByte
	case 'C':
		return tagChar
	case 'S':
		return tagShort
	case 'I':
		return tagInt
	case 'J':
		return tagLong
	case 'F':
		return tagFloat
	case 'D':
		return tagDouble
	case '[':
		return tagArray
	}
	return tagObject
}
