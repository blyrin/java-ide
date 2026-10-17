package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errFileExists = errors.New("文件已存在")

func createProjectFiles(name, path string, defaultCode bool) error {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" || path == "" {
		return errors.New("项目名称和保存路径不能为空")
	}
	if strings.ContainsAny(name, `/\`) {
		return errors.New("项目名称不能包含路径分隔符")
	}
	dir := filepath.Join(path, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		return err
	}
	mainClass := ""
	if defaultCode {
		mainClass = "Main"
		if err := os.WriteFile(filepath.Join(dir, "src", "Main.java"), []byte(defaultMainCode()), 0o644); err != nil {
			return err
		}
	}
	return writeProjectConfigure(dir, projectConfigure{MainClass: mainClass})
}

func projectConfigureFile(dir string) string { return filepath.Join(dir, ".ide.json") }

func writeProjectConfigure(dir string, cfg projectConfigure) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(projectConfigureFile(dir), b, 0o644)
}

func loadProjectConfigure(dir string) projectConfigure {
	var cfg projectConfigure
	b, err := os.ReadFile(projectConfigureFile(dir))
	if err != nil {
		return projectConfigure{}
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return projectConfigure{}
	}
	return cfg
}

func splitClassName(name string) (pkg, cls string) {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

func sourceFilePath(root, name string) string {
	pkg, cls := splitClassName(name)
	parts := append([]string{root, "src"}, strings.Split(pkg, ".")...)
	return filepath.Join(append(parts, cls+".java")...)
}

func createSourceFile(root, name string) error {
	file := sourceFilePath(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(file); err == nil {
		return errFileExists
	}
	pkg, cls := splitClassName(name)
	return os.WriteFile(file, []byte(defaultCode(cls, pkg)), 0o644)
}

func deleteSourceFile(root, name string) error {
	file := sourceFilePath(root, name)
	if _, err := os.Stat(file); err != nil {
		return errors.New("文件不存在")
	}
	return os.Remove(file)
}

func renameTarget(path, name string) string {
	return filepath.Join(filepath.Dir(path), name+filepath.Ext(path))
}

func renameFile(path, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\`) {
		return errors.New("文件名称不合法")
	}
	target := renameTarget(path, name)
	if filepath.Clean(target) == filepath.Clean(path) {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		return errFileExists
	}
	return os.Rename(path, target)
}

func defaultCode(className, packageName string) string {
	if packageName == "" {
		return "public class " + className + " {\n\n}"
	}
	return "package " + packageName + ";\n\npublic class " + className + " {\n\n}"
}

func defaultMainCode() string {
	return "public class Main {\n" +
		"    public static void main(String[] args) {\n" +
		"        System.out.println(\"Hello World!\");\n" +
		"    }\n" +
		"}\n"
}

func javaSourceFiles(root string) []string {
	var out []string
	src := filepath.Join(root, "src")
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != src && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".java") {
			out = append(out, p)
		}
		return nil
	})
	return out
}
