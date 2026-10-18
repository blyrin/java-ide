package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

type processResult struct {
	ExitCode int
	Output   string
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}

func newCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hideWindow(cmd)
	return cmd
}

func runCapture(cmd *exec.Cmd) processResult {
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			out = []byte(err.Error())
		}
		return processResult{ExitCode: exitCodeOf(err), Output: string(out)}
	}
	return processResult{ExitCode: 0, Output: string(out)}
}

func detectBuildTool(root string) string {
	for _, name := range []string{"pom.xml"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return buildMaven
		}
	}
	for _, name := range []string{"build.gradle", "build.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return buildGradle
		}
	}
	return buildJavac
}

func (s *session) buildToolName() string {
	tool := s.buildTool
	if tool == "" {
		tool = s.settings.BuildTool
	}
	if tool == "" || tool == buildAuto {
		return detectBuildTool(s.path)
	}
	return tool
}

func commandEnv(jdkHome string) []string {
	if strings.TrimSpace(jdkHome) == "" {
		return nil
	}
	return append(os.Environ(), "JAVA_HOME="+strings.TrimSpace(jdkHome))
}

func toolPath(jdkHome, name string) string {
	if home := strings.TrimSpace(jdkHome); home != "" {
		return filepath.Join(home, "bin", name)
	}
	return name
}

func javacBuild(projectPath, encoding, jdkHome string) processResult {
	sources := javaSourceFiles(projectPath)
	if len(sources) == 0 {
		return processResult{ExitCode: -1, Output: "src 目录下没有找到任何 Java 源文件"}
	}
	if encoding == "" {
		encoding = "UTF-8"
	}
	args := []string{"-J-Duser.language=en", "-J-Duser.country=US", "-encoding", encoding, "-g",
		"-s", projectPath, "-d", filepath.Join(projectPath, "out")}
	args = append(args, sources...)
	cmd := newCommand(toolPath(jdkHome, "javac"), args...)
	cmd.Dir = projectPath
	return runCapture(cmd)
}

func mavenBuild(projectPath, maven, jdkHome string, goals ...string) processResult {
	if len(goals) == 0 {
		goals = []string{"-q", "compile"}
	}
	if maven == "" {
		maven = "mvn"
	}
	cmd := newCommand(maven, goals...)
	cmd.Dir = projectPath
	cmd.Env = commandEnv(jdkHome)
	return runCapture(cmd)
}

func gradleBuild(projectPath, gradle, jdkHome string, tasks ...string) processResult {
	if len(tasks) == 0 {
		tasks = []string{"-q", "compileJava"}
	}
	if gradle == "" {
		gradle = "gradle"
	}
	cmd := newCommand(gradle, tasks...)
	cmd.Dir = projectPath
	cmd.Env = commandEnv(jdkHome)
	return runCapture(cmd)
}

func (s *session) toolBuild(tool string) processResult {
	st := s.settings.normalized()
	switch tool {
	case buildMaven:
		return mavenBuild(s.path, st.MavenCommand, st.JdkHome)
	case buildGradle:
		return gradleBuild(s.path, st.GradleCommand, st.JdkHome)
	default:
		encoding := s.encoding
		if encoding == "" {
			encoding = st.Encoding
		}
		return javacBuild(s.path, encoding, st.JdkHome)
	}
}

func (s *session) toolLabel(tool string) string {
	switch tool {
	case buildMaven:
		return "Maven"
	case buildGradle:
		return "Gradle"
	default:
		return "javac"
	}
}

func jarPackage(projectPath, mainClass, jdkHome string) processResult {
	jar := filepath.Join(projectPath, "out", filepath.Base(projectPath)+".jar")
	args := []string{"--create", "--file", jar}
	if mainClass != "" {
		args = append(args, "--main-class", mainClass)
	}
	args = append(args, "-C", filepath.Join(projectPath, "out"), ".")
	cmd := newCommand(toolPath(jdkHome, "jar"), args...)
	cmd.Dir = projectPath
	if res := runCapture(cmd); res.ExitCode != 0 {
		return res
	}
	return processResult{ExitCode: 0, Output: "已生成 " + jar}
}

func decompileClass(classFile, jdkHome string) processResult {
	cmd := newCommand(toolPath(jdkHome, "javap"), "-c", classFile)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return processResult{ExitCode: exitCodeOf(err), Output: err.Error()}
	}
	return processResult{ExitCode: exitCodeOf(err), Output: string(out)}
}

func gitTool(gitCommand string) string {
	if strings.TrimSpace(gitCommand) != "" {
		return strings.TrimSpace(gitCommand)
	}
	return "git"
}

func notifyBuild(title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body, Group: "build"})
	n.Show()
}
