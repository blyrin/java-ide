package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type runOptions struct {
	javaCommand string
	mainClass   string
	classpath   string
	vmArgs      []string
	programArgs []string
	env         []string
	workDir     string
	jdkHome     string
	debugPort   int
}

func buildProject(projectPath string) processResult {
	return javacBuild(projectPath, "UTF-8", "")
}

func gitFetch(url, branch, dir, gitCommand string) processResult {
	return runCapture(newCommand(gitTool(gitCommand), "clone", url, "-b", branch, dir))
}

type engine struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	closed bool
}

func (e *engine) run(projectPath string, opts runOptions, onOutput func(string)) processResult {
	java := strings.TrimSpace(opts.javaCommand)
	if java == "" {
		java = toolPath(opts.jdkHome, "java")
	}
	cp := opts.classpath
	if cp == "" {
		cp = filepath.Join(projectPath, "out")
	}
	args := []string{"-Dfile.encoding=UTF-8", "-Dstdout.encoding=UTF-8", "-Dstderr.encoding=UTF-8", "-cp", cp}
	if opts.debugPort > 0 {
		args = append(args, jdwpAgentArgs(opts.debugPort))
	}
	args = append(args, opts.vmArgs...)
	args = append(args, opts.mainClass)
	args = append(args, opts.programArgs...)
	cmd := newCommand(java, args...)
	cmd.Dir = opts.workDir
	if cmd.Dir == "" {
		cmd.Dir = projectPath
	}
	if len(opts.env) > 0 {
		cmd.Env = append(os.Environ(), opts.env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return processResult{ExitCode: -1, Output: err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return processResult{ExitCode: -1, Output: err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return processResult{ExitCode: -1, Output: err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return processResult{ExitCode: -1, Output: err.Error()}
	}
	e.mu.Lock()
	e.cmd, e.stdin, e.closed = cmd, stdin, false
	e.mu.Unlock()

	stream := func(r io.Reader) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			reader := bufio.NewReader(r)
			for {
				line, err := reader.ReadString('\n')
				if line != "" {
					onOutput(line)
				}
				if err != nil {
					if err != io.EOF && line == "" {
						onOutput(err.Error() + "\n")
					}
					return
				}
			}
		}()
		return done
	}
	outDone, errDone := stream(stdout), stream(stderr)
	<-outDone
	<-errDone
	code := exitCodeOf(cmd.Wait())

	e.mu.Lock()
	e.cmd, e.stdin = nil, nil
	e.mu.Unlock()
	return processResult{ExitCode: code}
}

func (e *engine) stop() {
	e.mu.Lock()
	cmd := e.cmd
	e.closed = true
	e.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func (e *engine) send(input string) {
	e.mu.Lock()
	stdin, closed := e.stdin, e.closed
	e.mu.Unlock()
	if stdin == nil || closed {
		return
	}
	io.WriteString(stdin, input)
}

func consoleLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.TrimSuffix(l, "\r"))
	}
	return out
}
