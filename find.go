package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	maxSearchMatches  = 5000
	maxSearchFileSize = 4 << 20
	searchWorkers     = 8
	searchLineContext = 48
)

type textSpan struct {
	start int
	end   int
}

type findMatch struct {
	file   string
	line   int
	col    int
	length int
	text   string
	hits   []textSpan
}

type findRow struct {
	file  string
	count int
	match *findMatch
}

func (r findRow) isHeader() bool { return r.match == nil }

type findFocus int

const (
	findFocusNone findFocus = iota
	findFocusQuery
)

type findState struct {
	query         string
	mask          string
	caseSensitive bool
	wholeWord     bool
	useRegex      bool
	scope         string
	results       []findMatch
	rows          []findRow
	files         int
	skipped       int
	running       bool
	searched      bool
	truncated     bool
	err           string
	seq           uint64
	cancel        chan struct{}
	list          ui.ListState
	sel           int
	opened        int
	focus         findFocus
}

var searchScopes = []string{"当前文件", "源码目录", "整个项目"}

var skipDirs = map[string]bool{
	".git": true, ".ide": true, ".gradle": true, ".idea": true, ".mygo": true,
	".settings": true, ".vscode": true, "out": true, "build": true, "target": true,
	"node_modules": true,
}

func looksBinary(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".class", ".jar", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf",
		".zip", ".gz", ".exe", ".dll", ".so", ".dylib", ".bin", ".o", ".obj":
		return true
	}
	return false
}

type findMatcher struct {
	query string
	rx    *regexp.Regexp
	word  bool
	fold  bool
}

func compileFindMatcher(query string, caseSensitive, wholeWord, useRegex bool) (*findMatcher, error) {
	m := &findMatcher{query: query, word: wholeWord, fold: !caseSensitive}
	if !useRegex {
		return m, nil
	}
	pat := query
	if wholeWord {
		pat = `\b(?:` + pat + `)\b`
	}
	if !caseSensitive {
		pat = `(?i)` + pat
	}
	rx, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	m.rx = rx
	return m, nil
}

func (m *findMatcher) find(line string) []textSpan {
	if m.rx != nil {
		var out []textSpan
		for _, loc := range m.rx.FindAllStringIndex(line, -1) {
			if loc[1] > loc[0] {
				out = append(out, textSpan{loc[0], loc[1]})
			}
		}
		return out
	}
	if m.query == "" {
		return nil
	}
	var out []textSpan
	if m.fold && (!isASCII(m.query) || !isASCII(line)) {
		out = foldSpans(line, m.query)
	} else {
		hay, needle := line, m.query
		if m.fold {
			hay, needle = asciiLower(line), asciiLower(m.query)
		}
		out = allSpans(hay, needle)
	}
	if m.word {
		out = slices.DeleteFunc(out, func(sp textSpan) bool { return !wholeWordAt(line, sp) })
	}
	return out
}

func allSpans(hay, needle string) []textSpan {
	if needle == "" {
		return nil
	}
	var out []textSpan
	for at := 0; at <= len(hay); {
		i := strings.Index(hay[at:], needle)
		if i < 0 {
			return out
		}
		start := at + i
		out = append(out, textSpan{start, start + len(needle)})
		at = start + len(needle)
	}
	return out
}

func foldSpans(line, needle string) []textSpan {
	lr, nr := []rune(line), []rune(needle)
	if len(nr) == 0 || len(lr) < len(nr) {
		return nil
	}
	offs := make([]int, len(lr)+1)
	for i, r := range lr {
		offs[i+1] = offs[i] + utf8.RuneLen(r)
	}
	lower := make([]rune, len(nr))
	for i, r := range nr {
		lower[i] = unicode.ToLower(r)
	}
	var out []textSpan
	for i := 0; i+len(nr) <= len(lr); {
		match := true
		for j := range nr {
			if unicode.ToLower(lr[i+j]) != lower[j] {
				match = false
				break
			}
		}
		if !match {
			i++
			continue
		}
		out = append(out, textSpan{offs[i], offs[i+len(nr)]})
		i += len(nr)
	}
	return out
}

func wholeWordAt(line string, sp textSpan) bool {
	if sp.start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(line[:sp.start]); isWordRune(r) {
			return false
		}
	}
	if sp.end < len(line) {
		if r, _ := utf8.DecodeRuneInString(line[sp.end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	changed := false
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			changed = true
			break
		}
	}
	if !changed {
		return s
	}
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

type fileMask []string

func parseFileMask(s string) fileMask {
	var out fileMask
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.ContainsAny(part, "*?[") {
			part = "*" + part + "*"
		}
		out = append(out, part)
	}
	return out
}

func (m fileMask) match(name string) bool {
	if len(m) == 0 {
		return true
	}
	for _, p := range m {
		if ok, err := filepath.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

func (m fileMask) err() error {
	for _, p := range m {
		if _, err := filepath.Match(p, "x"); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) searchRoot() string {
	switch s.find.scope {
	case searchScopes[0]:
		return s.currentPath
	case searchScopes[1]:
		return filepath.Join(s.path, "src")
	default:
		return s.path
	}
}

func (s *session) showFindPane() {
	s.sideTab = sideFind
	s.find.focus = findFocusQuery
	if s.ed == nil || s.ed.Buffer == nil {
		return
	}
	if text := strings.TrimSpace(s.ed.SelectedText()); text != "" && !strings.ContainsAny(text, "\n\r") {
		s.find.query = text
	}
}

func (s *session) cancelSearch() {
	if s.find.cancel != nil {
		close(s.find.cancel)
		s.find.cancel = nil
	}
	s.find.running = false
}

func (s *session) runSearch() {
	f := &s.find
	f.err = ""
	query := strings.TrimSpace(f.query)
	f.results, f.rows, f.files, f.skipped = nil, nil, 0, 0
	f.searched, f.running, f.truncated = false, false, false
	f.sel, f.opened = -1, -1
	if query == "" {
		return
	}
	m, err := compileFindMatcher(query, f.caseSensitive, f.wholeWord, f.useRegex)
	if err != nil {
		f.err = "正则表达式无效：" + err.Error()
		return
	}
	root := s.searchRoot()
	single, singleFile, singleText := "", "", ""
	if f.scope == searchScopes[0] {
		if root == "" {
			f.err = "没有打开的文件"
			return
		}
		single, singleFile, singleText = root, root, s.editorText()
	}
	mask := parseFileMask(f.mask)
	if single != "" {
		mask = nil
	} else if err := mask.err(); err != nil {
		f.err = "文件名掩码无效：" + err.Error()
		return
	}
	s.cancelSearch()
	cancel := make(chan struct{})
	f.cancel = cancel
	f.running = true
	f.seq++
	seq, app := f.seq, s.app
	go func() {
		matches, files, skipped, truncated := searchFiles(searchJob{
			root: root, single: single, singleFile: singleFile, singleText: singleText,
			mask: mask, matcher: m, cancel: cancel,
		})
		app.update(func() {
			if f.seq != seq {
				return
			}
			f.results, f.rows = matches, buildFindRows(matches)
			f.files, f.skipped, f.truncated = files, skipped, truncated
			f.searched, f.running = true, false
			f.sel, f.opened = -1, -1
		})
	}()
}

type searchJob struct {
	root       string
	single     string
	singleFile string
	singleText string
	mask       fileMask
	matcher    *findMatcher
	cancel     <-chan struct{}
}

func searchFiles(j searchJob) ([]findMatch, int, int, bool) {
	if j.matcher == nil {
		return nil, 0, 0, false
	}
	if j.single != "" {
		return matchText(j.singleFile, j.singleText, j.matcher), 1, 0, false
	}
	files := collectSearchFiles(j.root, j.mask)
	matches, skipped := searchFileList(files, j.matcher, j.cancel)
	slices.SortFunc(matches, func(a, b findMatch) int {
		if c := strings.Compare(a.file, b.file); c != 0 {
			return c
		}
		if a.line != b.line {
			return a.line - b.line
		}
		return a.col - b.col
	})
	truncated := false
	if len(matches) > maxSearchMatches {
		matches, truncated = matches[:maxSearchMatches], true
	}
	return matches, len(files), skipped, truncated
}

func collectSearchFiles(root string, mask fileMask) []string {
	if root == "" {
		return nil
	}
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || looksBinary(d.Name()) || !mask.match(d.Name()) {
			return nil
		}
		if st, err := d.Info(); err == nil && st.Size() > maxSearchFileSize {
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out
}

func searchFileList(files []string, m *findMatcher, cancel <-chan struct{}) ([]findMatch, int) {
	work := make(chan string)
	var mu sync.Mutex
	var all []findMatch
	skipped := 0
	var wg sync.WaitGroup
	for range min(searchWorkers, max(len(files), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				select {
				case <-cancel:
					continue
				default:
				}
				ms, ok := searchFile(p, m)
				if !ok {
					mu.Lock()
					skipped++
					mu.Unlock()
				}
				if len(ms) > 0 {
					mu.Lock()
					all = append(all, ms...)
					mu.Unlock()
				}
			}
		}()
	}
loop:
	for _, p := range files {
		select {
		case work <- p:
		case <-cancel:
			break loop
		}
	}
	close(work)
	wg.Wait()
	select {
	case <-cancel:
		return nil, 0
	default:
	}
	return all, skipped
}

func searchFile(path string, m *findMatcher) ([]findMatch, bool) {
	if st, err := os.Stat(path); err != nil {
		return nil, false
	} else if st.Size() > maxSearchFileSize {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	text := string(b)
	if !utf8.Valid(b) || strings.IndexByte(text, 0) >= 0 {
		return nil, false
	}
	return matchText(path, text, m), true
}

func matchText(file, text string, m *findMatcher) []findMatch {
	var out []findMatch
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		spans := m.find(line)
		if len(spans) == 0 {
			continue
		}
		for _, sp := range spans {
			out = append(out, findMatch{
				file: file, line: i + 1, col: sp.start, length: sp.end - sp.start,
				text: line, hits: spans,
			})
		}
	}
	return out
}

func buildFindRows(matches []findMatch) []findRow {
	if len(matches) == 0 {
		return nil
	}
	var rows []findRow
	for i := range matches {
		if i == 0 || matches[i].file != matches[i-1].file {
			count := 1
			for j := i + 1; j < len(matches) && matches[j].file == matches[i].file; j++ {
				count++
			}
			rows = append(rows, findRow{file: matches[i].file, count: count})
		}
		rows = append(rows, findRow{match: &matches[i]})
	}
	return rows
}

func (s *session) findStep(d int) {
	f := &s.find
	if len(f.rows) == 0 {
		return
	}
	i := f.sel
	if i < 0 || i >= len(f.rows) {
		if d < 0 {
			i = len(f.rows)
		} else {
			i = -1
		}
	}
	for range len(f.rows) {
		i += d
		if i < 0 {
			i = len(f.rows) - 1
		}
		if i >= len(f.rows) {
			i = 0
		}
		if f.rows[i].match != nil {
			f.sel = i
			f.list.ScrollIntoView(i)
			return
		}
	}
}

func (a *app) openFindMatch(m *findMatch) {
	s := a.sess
	if s == nil || m == nil {
		return
	}
	a.openFileAt(m.file, m.line, m.col+1)
	if s.ed == nil || s.ed.Buffer == nil || m.length <= 0 || filepath.Clean(s.currentPath) != filepath.Clean(m.file) {
		return
	}
	start := s.ed.Buffer.Offset(editor.Pos{Line: m.line - 1, Col: m.col})
	s.ed.Select(start, min(start+m.length, s.ed.Buffer.Len()))
}

func (s *session) findPane(c *ui.Context) {
	t := c.Theme()
	f := &s.find
	bar, foot := t.Space(9), t.Space(9)
	ui.Box(c).Fill().Children(func() {
		scrollPane(c, bar, foot+t.Space(2.5), func() {
			paneHeader(c, searchIcon, "查找", func() {
				if f.running {
					ui.Spinner(c)
				}
			})
		}, func() {
			ui.Column(c).Gap(t.Space(1.5)).Padding(t.Space(0.5), t.Space(1.5)).Children(func() {
				in := ui.SearchField(c, &f.query).Label("查找内容").FillWidth().
					FocusBind(&f.focus, findFocusQuery)
				if in.Submitted() {
					s.runSearch()
				}
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, "范围").FontSize(t.Rem(0.85)).TextColor(t.TextMuted)
					scope := f.scope
					if scope == "" {
						scope = searchScopes[len(searchScopes)-1]
					}
					ui.Select(c, &scope, searchScopes).Width(t.Space(20)).Label("查找范围")
					f.scope = scope
					ui.Text(c, "文件名").FontSize(t.Rem(0.85)).TextColor(t.TextMuted)
					mask := ui.TextInput(c, &f.mask).Placeholder("*.java,*.xml").Width(t.Space(16)).Label("文件名过滤")
					if mask.Submitted() {
						s.runSearch()
					}
				})
				ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Checkbox(c, &f.caseSensitive, "区分大小写")
					ui.Checkbox(c, &f.wholeWord, "全字匹配")
					ui.Checkbox(c, &f.useRegex, "正则表达式")
					ui.Box(c).Grow(1)
					if f.running {
						if textButton(c, "取消", buttonPlain, closeIcon).Clicked() {
							s.cancelSearch()
						}
					} else if textButton(c, "查找", buttonAccent, searchIcon).Clicked() {
						s.runSearch()
					}
				})
				switch {
				case f.err != "":
					ui.Text(c, f.err).FontSize(t.Rem(0.8)).TextColor(t.Danger)
				case f.searched && len(f.results) == 0:
					ui.Text(c, "没有找到匹配项").TextColor(t.TextMuted)
				case len(f.results) > 0:
					ui.Text(c, "共 "+strconv.Itoa(len(f.results))+" 处匹配，"+strconv.Itoa(f.files)+" 个文件").
						FontSize(t.Rem(0.85)).TextColor(t.TextMuted)
					if f.truncated {
						ui.Text(c, "结果过多，仅显示前 "+strconv.Itoa(maxSearchMatches)+" 处").
							FontSize(t.Rem(0.75)).TextColor(t.Warning)
					}
					if f.skipped > 0 {
						ui.Text(c, "已跳过 "+strconv.Itoa(f.skipped)+" 个二进制或无法解码的文件").
							FontSize(t.Rem(0.75)).TextColor(t.TextMuted)
					}
				}
			})
			s.findResults(c)
		})
	})
	c.OnShortcut(0, ui.KeyF3, func() { s.findStep(1) })
	c.OnShortcut(ui.Shift, ui.KeyF3, func() { s.findStep(-1) })
}

func (s *session) findResults(c *ui.Context) {
	t := c.Theme()
	f := &s.find
	if len(f.rows) == 0 {
		return
	}
	f.list.Selected = &f.sel
	f.list.Label = func(i int) string {
		if m := f.rows[i].match; m != nil {
			return m.text
		}
		return ""
	}
	f.list.Key = func(i int) any {
		row := f.rows[i]
		if row.match != nil {
			return "m:" + row.match.file + ":" + strconv.Itoa(row.match.line) + ":" + strconv.Itoa(row.match.col)
		}
		return "h:" + row.file
	}
	f.list.Header = func(i int) bool { return f.rows[i].isHeader() }
	ui.List(c, &f.list, len(f.rows)).Grow(1).FillWidth().Gap(t.Space(0.5)).
		Rows(func(r ui.ListRow) {
			row := f.rows[r.Index]
			if row.isHeader() {
				head := ui.Row(r.Context).FillWidth().Gap(t.Space(1.5)).Padding(t.Space(0.5), t.Space(1)).
					AlignItems(ui.Center).Background(t.Background.Mix(t.Text, 0.07)).Radius(t.Radius)
				head.Children(func() {
					ui.Icon(r.Context, fileIcon).FontSize(iconSize(t, 0.85)).TextColor(t.TextMuted)
					ui.RichText(r.Context,
						ui.Span{Text: relativeTo(s.path, row.file), Weight: 600},
						ui.Span{Text: "  " + strconv.Itoa(row.count), Color: t.TextMuted},
					).FontSize(t.Rem(0.8)).SingleLine().Ellipsis("…").Grow(1)
				})
				return
			}
			match := row.match
			path := match.file
			item := ui.Row(r.Context).FillWidth().Gap(t.Space(1.5)).Padding(t.Space(0.5), t.Space(1)).
				AlignItems(ui.Center).Radius(t.Radius)
			if r.Selected() || r.ListFocused() {
				item.Background(activeOf(t))
			}
			item.ContextMenu(func(m *ui.Menu) {
				if m.Item("打开").Chosen() {
					s.later(func() { s.app.openFindMatch(match) })
				}
				m.Separator()
				if m.Item("复制路径").Chosen() {
					s.app.copyToClipboard(path)
				}
				if m.Item("在文件管理器中显示").Chosen() {
					mygo.Shell.ShowItemInFolder(path)
				}
			})
			item.Children(func() {
				ui.Text(r.Context, strconv.Itoa(match.line)).FontSize(t.Rem(0.75)).TextColor(t.TextMuted).
					MinWidth(t.Space(4.5)).SingleLine()
				ui.RichText(r.Context, matchSpans(t, match)...).FontSize(t.Rem(0.85)).
					SingleLine().Ellipsis("…").Grow(1)
			})
		})
	if f.sel >= 0 && f.sel < len(f.rows) && f.sel != f.opened {
		if m := f.rows[f.sel].match; m != nil {
			f.opened = f.sel
			s.later(func() { s.app.openFindMatch(m) })
		}
	}
}

func matchSpans(t *ui.Theme, m *findMatch) []ui.Span {
	lead := len(m.text) - len(strings.TrimLeft(m.text, " \t"))
	body := m.text[lead:]
	runes := []rune(body)
	ownA := max(0, min(m.col-lead, len(body)))
	ownB := max(ownA, min(m.col-lead+m.length, len(body)))
	ai := utf8.RuneCountInString(body[:ownA])
	bi := utf8.RuneCountInString(body[:ownB])
	lo, hi := max(0, ai-searchLineContext), min(len(runes), bi+searchLineContext)
	mark := make([]bool, hi-lo)
	for _, sp := range m.hits {
		a, b := max(sp.start-lead, 0), min(sp.end-lead, len(body))
		if a >= b {
			continue
		}
		ra, rb := utf8.RuneCountInString(body[:a]), utf8.RuneCountInString(body[:b])
		for i := max(ra, lo); i < min(rb, hi); i++ {
			mark[i-lo] = true
		}
	}
	muted, hit := ui.Span{Color: t.TextMuted}, ui.Span{Color: t.Text, Background: t.Accent.Mix(t.Background, 0.32)}
	var spans []ui.Span
	if lo > 0 {
		spans = append(spans, ui.Span{Text: "…", Color: t.TextMuted})
	}
	for i := lo; i < hi; {
		j := i + 1
		for j < hi && mark[j-lo] == mark[i-lo] {
			j++
		}
		if mark[i-lo] {
			hit.Text = string(runes[i:j])
			spans = append(spans, hit)
		} else {
			muted.Text = string(runes[i:j])
			spans = append(spans, muted)
		}
		i = j
	}
	if hi < len(runes) {
		spans = append(spans, ui.Span{Text: "…", Color: t.TextMuted})
	}
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: "", Color: t.TextMuted})
	}
	return spans
}
