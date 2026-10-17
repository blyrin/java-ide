package main

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

type buildError struct {
	file string
	line int
	col  int
	msg  string
	warn bool
}

var todoRe = regexp.MustCompile(`(//|/\*|\*)\s*(TODO|FIXME|XXX)\b[:\s]*(.*)`)

func todosIn(text, file string) []buildError {
	var out []buildError
	for i, line := range strings.Split(text, "\n") {
		m := todoRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		msg := strings.TrimSpace(m[3])
		if msg == "" {
			msg = m[2]
		}
		out = append(out, buildError{file: file, line: i + 1, msg: m[2] + " " + msg})
	}
	return out
}

func (s *session) refreshProblems() {
	if s.ed == nil || s.ed.Buffer == nil || s.currentPath == "" {
		return
	}
	text := s.editorText()
	var ds []editor.Diagnostic
	for _, e := range s.problemsFor(s.currentPath) {
		start := s.ed.Buffer.Offset(editor.Pos{Line: max(e.line-1, 0), Col: max(e.col-1, 0)})
		severity := editor.SeverityError
		if e.warn {
			severity = editor.SeverityWarning
		}
		ds = append(ds, editor.Diagnostic{
			Start: start, End: min(start+1, len(text)), Severity: severity, Message: e.msg, Source: "javac",
		})
	}
	s.ed.SetDiagnostics(s.ed.Buffer.Version(), ds)
}

func (s *session) problemsFor(file string) []buildError {
	file = filepath.Clean(file)
	var out []buildError
	for _, e := range s.buildErrors {
		if filepath.Clean(e.file) == file {
			out = append(out, e)
		}
	}
	for _, e := range s.todos {
		if filepath.Clean(e.file) == file {
			out = append(out, e)
		}
	}
	return out
}

func (s *session) allProblems() []buildError {
	out := make([]buildError, 0, len(s.buildErrors)+len(s.todos))
	out = append(out, s.buildErrors...)
	out = append(out, s.todos...)
	return out
}
