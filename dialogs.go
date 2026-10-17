package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	inputCreateFile = iota + 1
	inputDeleteFile
	inputCreateDir
	inputRename
)

type dialogs struct {
	createOpen        bool
	createName        string
	createPath        string
	createDefaultCode bool
	createGit         bool
	gitOpen           bool
	gitURL            string
	gitBranch         string
	gitDir            string
	gitBusy           bool

	configOpen      bool
	configMainClass string
	configJavaCmd   string

	runOpen        bool
	runIndex       int
	runName        string
	runMainClass   string
	runVMArgs      string
	runProgramArgs string
	runEnv         string
	runWorkDir     string
	runBuildTool   string

	evalOpen bool
	evalExpr string
	evalText string
	evalErr  bool
	evalBusy bool

	condOpen  bool
	condFile  string
	condLine  int
	condExpr  string
	condTitle string

	settingsOpen   bool
	settingsValues appSettings
	settingsTheme  string
	settingsBuild  string
	settingsMono   string

	errorOpen bool
	errorText string

	inputOpen  bool
	inputMode  int
	inputTitle string
	inputLabel string
	inputValue string

	renamePath    string
	renameProject bool
	deleteFile    bool
	deletePath    string
	deleteName    string

	deleteOpen bool
	deleteProj string

	aboutOpen bool

	autoFocus  bool
	inputBound bool
	firstInput ui.Handle
}

func (d *dialogs) reset() { *d = dialogs{} }

func (a *app) openCreateDialog() {
	d := &a.dlg
	d.reset()
	d.createOpen = true
	d.autoFocus = true
}

func (a *app) openGitDialog() {
	d := &a.dlg
	d.reset()
	d.gitOpen = true
	d.autoFocus = true
}

func (a *app) openConfigDialog() {
	s := a.sess
	if s == nil {
		return
	}
	d := &a.dlg
	d.reset()
	d.configMainClass = s.mainClass
	d.configJavaCmd = s.javaCommand
	d.configOpen = true
	d.autoFocus = true
}

func (a *app) openRunDialog() {
	s := a.sess
	if s == nil {
		return
	}
	d := &a.dlg
	d.reset()
	d.runOpen = true
	d.runIndex = s.activeRun
	rc := s.activeRunConfig()
	d.runName = rc.Name
	d.runMainClass = rc.MainClass
	d.runVMArgs = strings.Join(rc.VMArgs, " ")
	d.runProgramArgs = strings.Join(rc.ProgramArgs, " ")
	d.runEnv = strings.Join(rc.Env, "\n")
	d.runWorkDir = rc.WorkDir
	d.runBuildTool = rc.BuildTool
	d.autoFocus = true
}

func (a *app) openEvaluateDialog() {
	s := a.sess
	if s == nil || s.dbg == nil {
		a.showError("请先开始调试会话")
		return
	}
	d := &a.dlg
	d.evalOpen = true
	d.evalExpr = ""
	d.evalText = ""
	d.evalErr = false
	d.evalBusy = false
	d.autoFocus = true
}

func (a *app) runEvaluateExpression() {
	s := a.sess
	if s == nil || s.dbg == nil {
		return
	}
	expr := strings.TrimSpace(a.dlg.evalExpr)
	if expr == "" {
		return
	}
	a.update(func() {
		a.dlg.evalBusy = true
		a.dlg.evalText = ""
		a.dlg.evalErr = false
	})
	s.dbg.evaluate(expr, func(text string, err error) {
		a.dlg.evalBusy = false
		if err != nil {
			a.dlg.evalErr = true
			a.dlg.evalText = err.Error()
			return
		}
		a.dlg.evalText = text
	})
}

func (a *app) openConditionDialog(bp debugBreakpoint) {
	d := &a.dlg
	d.condOpen = true
	d.condFile = bp.File
	d.condLine = bp.Line
	d.condExpr = bp.Condition
	d.condTitle = filepath.Base(bp.File) + ":" + strconv.Itoa(bp.Line)
	d.autoFocus = true
}

func (a *app) saveBreakpointCondition() {
	s := a.sess
	if s == nil {
		return
	}
	s.setBreakpointCondition(a.dlg.condFile, a.dlg.condLine, strings.TrimSpace(a.dlg.condExpr))
	a.dlg.condOpen = false
}

func (a *app) openWatchDialog() {
	s := a.sess
	if s == nil || s.dbg == nil {
		a.showError("请先开始调试会话")
		return
	}
	d := &a.dlg
	d.evalOpen = true
	d.evalExpr = ""
	d.evalText = ""
	d.evalErr = false
	d.evalBusy = false
	d.autoFocus = true
}

func (a *app) addWatchFromDialog() {
	s := a.sess
	if s == nil || s.dbg == nil {
		return
	}
	expr := strings.TrimSpace(a.dlg.evalExpr)
	if expr == "" {
		return
	}
	s.dbg.addWatch(expr)
	a.dlg.evalOpen = false
}

func (a *app) openInputDialog(mode int, title, label string) {
	d := &a.dlg
	d.inputOpen = true
	d.inputMode = mode
	d.inputTitle = title
	d.inputLabel = label
	d.inputValue = ""
	d.autoFocus = true
}

func (a *app) openRenameDialog(path string) {
	d := &a.dlg
	d.inputOpen = true
	d.inputMode = inputRename
	d.inputTitle = "重命名文件"
	d.inputLabel = "新的文件名称"
	d.inputValue = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	d.renamePath = path
	d.autoFocus = true
}

func (a *app) showError(text string) {
	a.dlg.errorText = text
	a.dlg.errorOpen = true
}

func (a *app) chooseDirectory(target *string, parent *mygo.Window) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:     "请选择一个目录",
		Directory: true,
		Parent:    parent,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	*target = paths[0]
}

func (a *app) createProject() {
	d := &a.dlg
	name := strings.TrimSpace(d.createName)
	path := strings.TrimSpace(d.createPath)
	if name == "" || path == "" {
		return
	}
	if err := createProjectFiles(name, path, d.createDefaultCode); err != nil {
		a.showError("未知错误，创建项目失败：" + err.Error())
		return
	}
	d.createOpen = false
	if d.createGit {
		root := filepath.Join(path, name)
		go func() {
			if err := gitInitRepo(root, a.cfg.Settings.GitCommand); err != nil {
				a.update(func() { a.showError("创建 Git 仓库失败：" + err.Error()) })
			}
		}()
	}
	a.openProject(name, filepath.Join(path, name))
}

func (a *app) gitFetch() {
	d := &a.dlg
	if d.gitBusy {
		return
	}
	url := strings.TrimSpace(d.gitURL)
	branch := strings.TrimSpace(d.gitBranch)
	dir := strings.TrimSpace(d.gitDir)
	if url == "" || branch == "" || dir == "" {
		return
	}
	d.gitBusy = true
	go func() {
		res := gitFetch(url, branch, dir, a.cfg.Settings.GitCommand)
		a.update(func() {
			d.gitBusy = false
			if res.ExitCode != 0 {
				a.showError(strings.TrimSpace(res.Output))
				return
			}
			d.gitOpen = false
			a.openProject(filepath.Base(filepath.Clean(dir)), dir)
		})
	}()
}

func (a *app) saveProjectConfig() {
	s := a.sess
	if s == nil {
		return
	}
	s.mainClass = strings.TrimSpace(a.dlg.configMainClass)
	s.javaCommand = strings.TrimSpace(a.dlg.configJavaCmd)
	s.saveConfigure()
	if rc := s.activeRunConfig(); rc.MainClass != s.mainClass {
		s.setRunConfig(runConfig{Name: rc.Name, MainClass: s.mainClass})
	}
	a.dlg.configOpen = false
}

func (a *app) saveRunConfig() {
	s := a.sess
	if s == nil {
		return
	}
	d := &a.dlg
	rc := runConfig{
		Name:        strings.TrimSpace(d.runName),
		MainClass:   strings.TrimSpace(d.runMainClass),
		VMArgs:      splitFields(d.runVMArgs),
		ProgramArgs: splitFields(d.runProgramArgs),
		Env:         splitLines(d.runEnv),
		WorkDir:     strings.TrimSpace(d.runWorkDir),
		BuildTool:   d.runBuildTool,
	}
	if rc.Name == "" {
		rc.Name = "未命名配置"
	}
	if d.runIndex >= 0 && d.runIndex < len(s.runs) {
		s.runs[d.runIndex] = rc
		s.activeRun = d.runIndex
	} else {
		s.runs = append(s.runs, rc)
		s.activeRun = len(s.runs) - 1
	}
	if rc.MainClass != "" {
		s.mainClass = rc.MainClass
	}
	s.saveConfigure()
	d.runOpen = false
}

func splitFields(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		out = append(out, f)
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (a *app) submitInput() {
	d := &a.dlg
	name := strings.TrimSpace(d.inputValue)
	mode := d.inputMode
	d.inputOpen = false
	if name == "" {
		return
	}
	if mode == inputRename && d.renameProject {
		d.renameProject = false
		a.cfg.rename(d.renamePath, name)
		a.cfg.save()
		d.renamePath = ""
		return
	}
	s := a.sess
	if s == nil {
		return
	}
	switch mode {
	case inputCreateFile:
		if err := createSourceFile(s.path, name); err != nil {
			a.showError("创建文件失败：" + err.Error())
			return
		}
		s.refreshTree()
		a.openFile(sourceFilePath(s.path, name))
	case inputCreateDir:
		dir := filepath.Join(s.path, filepath.FromSlash(name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.showError("创建目录失败：" + err.Error())
			return
		}
		s.refreshTree()
	case inputRename:
		if err := renameFile(a.dlg.renamePath, name); err != nil {
			a.showError("重命名失败：" + err.Error())
			return
		}
		if filepath.Clean(s.currentPath) == filepath.Clean(a.dlg.renamePath) {
			next := renameTarget(a.dlg.renamePath, name)
			s.closeTab(s.active)
			a.openFile(next)
		}
		s.refreshTree()
	case inputDeleteFile:
		file := sourceFilePath(s.path, name)
		if err := deleteSourceFile(s.path, name); err != nil {
			a.showError("删除文件失败：" + err.Error())
			return
		}
		if filepath.Clean(s.currentPath) == filepath.Clean(file) {
			s.currentPath = ""
			s.setEditor("", "", true)
		}
		s.refreshTree()
	}
	a.dlg.renamePath = ""
}

func (a *app) welcomeDialogs(c *ui.Context) {
	a.createProjectDialog(c)
	a.gitProjectDialog(c)
	a.deleteProjectDialog(c)
	a.settingsDialog(c)
	a.aboutDialog(c)
	a.errorDialog(c)
}

func (a *app) confirmDeleteProject(p projectEntity) {
	if !a.cfg.Settings.ConfirmDelete {
		a.deleteProject()
		return
	}
	a.dlg.deleteOpen = true
	a.dlg.deleteProj = p.FilePath
}

func (a *app) confirmDeleteFile(path string) {
	a.dlg.deleteFile = true
	a.dlg.deletePath = path
	a.dlg.deleteName = filepath.Base(path)
}

func (a *app) deleteProject() {
	d := &a.dlg
	if d.deleteProj == "" {
		return
	}
	os.RemoveAll(d.deleteProj)
	a.cfg.remove(d.deleteProj)
	a.cfg.save()
	d.deleteOpen = false
	d.deleteProj = ""
}

func (a *app) deleteFile() {
	d := &a.dlg
	if d.deletePath == "" {
		return
	}
	if err := os.RemoveAll(d.deletePath); err != nil {
		a.showError("删除失败：" + err.Error())
		return
	}
	if s := a.sess; s != nil {
		for i := len(s.tabs) - 1; i >= 0; i-- {
			if filepath.Clean(s.tabs[i].path) == filepath.Clean(d.deletePath) {
				s.closeTab(i)
			}
		}
		s.refreshTree()
	}
	d.deleteFile = false
	d.deletePath, d.deleteName = "", ""
}

func (a *app) mainDialogs(c *ui.Context) {
	a.configDialog(c)
	a.evaluateDialog(c)
	a.conditionDialog(c)
	a.runConfigDialog(c)
	a.inputDialog(c)
	a.settingsDialog(c)
	a.aboutDialog(c)
	a.deleteFileDialog(c)
	a.errorDialog(c)
}

func (a *app) dialog(c *ui.Context, open *bool, width, height float32, fn func()) {
	t := c.Theme()
	w, h := c.Size()
	a.dlg.firstInput = ui.Handle{}
	a.dlg.inputBound = false
	ui.DialogBase(c, open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.34))
		panel.Padding(t.Space(3), t.Space(3.5)).Gap(t.Space(2)).
			Background(t.Surface).Border(1, t.Border).Radius(t.Radius).
			Shadow(0, t.Space(2), t.Space(9), -t.Space(1), shadowOf(t)).
			Width(width).MaxWidth(w - t.Space(8)).MaxHeight(h - t.Space(8))
		if height > 0 {
			panel.Height(height + t.Space(8) + 2)
		}
		fn()
	})
	if a.dlg.autoFocus && a.dlg.inputBound {
		a.dlg.firstInput.Focus(c)
		a.dlg.autoFocus = false
	}
}

func (a *app) dialogField(c *ui.Context, value *string, placeholder, label string) ui.Element {
	in := ui.TextInput(c, value).Placeholder(placeholder).Label(label).FillWidth()
	if a.dlg.autoFocus && !a.dlg.inputBound {
		in.Bind(&a.dlg.firstInput)
		a.dlg.inputBound = true
	}
	return in
}

func (a *app) dialogFieldArea(c *ui.Context, value *string, placeholder, label string) ui.Element {
	in := ui.TextArea(c, value).Placeholder(placeholder).Label(label).FillWidth()
	if a.dlg.autoFocus && !a.dlg.inputBound {
		in.Bind(&a.dlg.firstInput)
		a.dlg.inputBound = true
	}
	return in
}

func dialogTitle(c *ui.Context, icon *ui.SVG, title string, rem float32) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2.5)).AlignItems(ui.Center).Children(func() {
		badge(c, icon, t.Rem(2))
		ui.Text(c, title).FontSize(t.Rem(rem)).Bold().Grow(1)
	})
}

func dialogButtons(c *ui.Context, fn func()) {
	ui.Row(c).Shrink(0).Gap(c.Theme().Space(1.5)).Justify(ui.End).AlignItems(ui.Center).Children(fn)
}

func (a *app) deleteProjectDialog(c *ui.Context) {
	d := &a.dlg
	if !d.deleteOpen {
		return
	}
	name := d.deleteProj
	for _, p := range a.cfg.Projects {
		if p.FilePath == d.deleteProj {
			name = p.Name
		}
	}
	a.dialog(c, &d.deleteOpen, 340, 0, func() {
		t := c.Theme()
		dialogTitle(c, trashIcon, "删除项目", 1.15)
		ui.Text(c, "你确定要删除 “"+name+"” 吗？项目文件将一并删除，且不可恢复。").TextColor(t.TextMuted)
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.deleteOpen = false
			}
			if textButton(c, "删除", buttonDanger, trashIcon).Clicked() {
				a.deleteProject()
			}
		})
	})
}

func (a *app) deleteFileDialog(c *ui.Context) {
	d := &a.dlg
	if !d.deleteFile {
		return
	}
	a.dialog(c, &d.deleteFile, 340, 0, func() {
		t := c.Theme()
		dialogTitle(c, trashIcon, "删除文件", 1.15)
		ui.Text(c, "你确定要删除 “"+d.deleteName+"” 吗？此操作不可恢复。").TextColor(t.TextMuted)
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.deleteFile = false
			}
			if textButton(c, "删除", buttonDanger, trashIcon).Clicked() {
				a.deleteFile()
			}
		})
	})
}

func (a *app) aboutDialog(c *ui.Context) {
	d := &a.dlg
	if !d.aboutOpen {
		return
	}
	a.dialog(c, &d.aboutOpen, 360, 0, func() {
		t := c.Theme()
		dialogTitle(c, javaIcon, "关于", 1.2)
		ui.Text(c, "用 MyGo 原生 UI 构建的 Java 集成开发环境").TextColor(t.TextMuted)
		ui.Text(c, "版本 "+appVersion+" · MyGo "+mygoVersion).FontSize(t.Rem(0.85)).TextColor(t.TextMuted)
		dialogButtons(c, func() {
			if textButton(c, "确定", buttonAccent, nil).Clicked() {
				d.aboutOpen = false
			}
		})
	})
}

func (a *app) createProjectDialog(c *ui.Context) {
	d := &a.dlg
	if !d.createOpen {
		return
	}
	a.dialog(c, &d.createOpen, 420, 0, func() {
		t := c.Theme()
		dialogTitle(c, plusIcon, "创建新的项目", 1.3)
		ui.Column(c).Gap(t.Space(2.5)).Children(func() {
			ui.Field(c, "项目名称", func() {
				a.dialogField(c, &d.createName, "例如：MyJavaApp", "项目名称")
			})
			ui.Field(c, "项目路径", func() {
				ui.Row(c).Gap(t.Space(1.5)).Children(func() {
					a.dialogField(c, &d.createPath, "例如：D:\\workspace", "项目路径")
					if iconButton(c, "选择目录", openIcon).Clicked() {
						a.chooseDirectory(&d.createPath, a.welcome)
					}
				})
			}).Description("保存位置：" + filepath.Join(d.createPath, d.createName))
			ui.Column(c).Gap(t.Space(1)).Children(func() {
				ui.Checkbox(c, &d.createDefaultCode, "生成默认代码")
				ui.Checkbox(c, &d.createGit, "创建 Git 仓库")
			})
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.createOpen = false
			}
			create := textButton(c, "创建项目", buttonAccent, plusIcon)
			if strings.TrimSpace(d.createName) == "" || strings.TrimSpace(d.createPath) == "" {
				create.Disabled(true).Tooltip("请先填写上述配置信息")
			}
			if create.Clicked() {
				a.createProject()
			}
		})
	})
}

func (a *app) gitProjectDialog(c *ui.Context) {
	d := &a.dlg
	if !d.gitOpen {
		return
	}
	a.dialog(c, &d.gitOpen, 440, 0, func() {
		t := c.Theme()
		dialogTitle(c, gitIcon, "从 Git 获取项目", 1.3)
		ui.Column(c).Gap(t.Space(2.5)).Children(func() {
			ui.Field(c, "远程地址", func() {
				a.dialogField(c, &d.gitURL, "git@github.com:user/repo.git", "远程地址")
			})
			ui.Field(c, "远程分支", func() {
				a.dialogField(c, &d.gitBranch, "main", "远程分支")
			})
			ui.Field(c, "项目路径", func() {
				ui.Row(c).Gap(t.Space(1.5)).Children(func() {
					a.dialogField(c, &d.gitDir, "必须是一个已存在的空目录", "项目路径")
					if iconButton(c, "选择目录", openIcon).Clicked() {
						a.chooseDirectory(&d.gitDir, a.welcome)
					}
				})
			})
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.gitOpen = false
			}
			start := textButton(c, map[bool]string{true: "正在获取...", false: "开始获取"}[d.gitBusy],
				buttonAccent, gitIcon)
			ready := strings.TrimSpace(d.gitURL) != "" && strings.TrimSpace(d.gitBranch) != "" && strings.TrimSpace(d.gitDir) != ""
			if !ready || d.gitBusy {
				start.Disabled(true)
			}
			if start.Clicked() {
				a.gitFetch()
			}
		})
	})
}

func (a *app) configDialog(c *ui.Context) {
	d := &a.dlg
	if !d.configOpen {
		return
	}
	a.dialog(c, &d.configOpen, 420, 0, func() {
		t := c.Theme()
		dialogTitle(c, gearIcon, "项目配置", 1.3)
		ui.Column(c).Gap(t.Space(2.5)).Children(func() {
			ui.Field(c, "主类", func() {
				a.dialogField(c, &d.configMainClass, "com.test.Main", "主类")
			})
			ui.Field(c, "Java 位置", func() {
				a.dialogField(c, &d.configJavaCmd, "java", "Java 位置")
			}).Description("留空则使用 PATH 中的 java")
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.configOpen = false
			}
			if textButton(c, "确定", buttonAccent, nil).Clicked() {
				a.saveProjectConfig()
			}
		})
	})
}

func (a *app) runConfigDialog(c *ui.Context) {
	d := &a.dlg
	s := a.sess
	if !d.runOpen || s == nil {
		return
	}
	a.dialog(c, &d.runOpen, 460, 0, func() {
		t := c.Theme()
		dialogTitle(c, playIcon, "编辑运行配置", 1.3)
		ui.Scroll(c).Grow(1).FillWidth().MaxHeight(t.Rem(26)).Children(func() {
			ui.Column(c).Gap(t.Space(2.5)).Children(func() {
				ui.Field(c, "配置名称", func() {
					a.dialogField(c, &d.runName, "配置名称，如 Main", "配置名称")
				})
				ui.Field(c, "主类", func() {
					a.dialogField(c, &d.runMainClass, "主类，如 com.test.Main", "主类")
				})
				ui.Field(c, "程序参数", func() {
					a.dialogField(c, &d.runProgramArgs, "程序参数，以空格分隔", "程序参数")
				})
				ui.Field(c, "虚拟机参数", func() {
					a.dialogField(c, &d.runVMArgs, "虚拟机参数，以空格分隔", "虚拟机参数")
				})
				ui.Field(c, "环境变量", func() {
					a.dialogFieldArea(c, &d.runEnv, "环境变量，每行一个 KEY=VALUE", "环境变量").
						Grow(0).MaxHeight(t.Rem(5))
				})
				ui.Field(c, "工作目录", func() {
					ui.Row(c).Gap(t.Space(1.5)).Children(func() {
						a.dialogField(c, &d.runWorkDir, "工作目录，留空为项目根目录", "工作目录")
						if iconButton(c, "选择目录", openIcon).Clicked() {
							a.chooseDirectory(&d.runWorkDir, a.win)
						}
					})
				})
				ui.Field(c, "构建工具", func() {
					ui.Select(c, &d.runBuildTool, []string{"构建：跟随项目", "构建：javac", "构建：Maven", "构建：Gradle"}).
						Width(t.Space(30)).Label("构建工具")
				})
			})
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.runOpen = false
			}
			if s.activeRun >= 0 && s.activeRun < len(s.runs) && len(s.runs) > 1 {
				if textButton(c, "删除此配置", buttonDanger, trashIcon).Clicked() {
					s.removeRunConfig()
					d.runOpen = false
				}
			}
			if textButton(c, "保存", buttonAccent, nil).Clicked() {
				a.saveRunConfig()
			}
		})
	})
}

func (a *app) evaluateDialog(c *ui.Context) {
	d := &a.dlg
	if !d.evalOpen {
		return
	}
	a.dialog(c, &d.evalOpen, 480, 0, func() {
		t := c.Theme()
		dialogTitle(c, eyeIcon, "计算表达式", 1.2)
		ui.Column(c).Gap(t.Space(2.5)).Children(func() {
			ui.Field(c, "表达式", func() {
				in := a.dialogField(c, &d.evalExpr, "例如：list.size() 或 count + 1", "表达式")
				_ = in
			})
			ui.Field(c, "结果", func() {
				out := ui.TextArea(c, &d.evalText).ReadOnly(true).Grow(0).FillWidth().MinHeight(0).
					MaxHeight(t.Rem(5)).Label("计算结果")
				if d.evalErr {
					out.TextColor(t.Danger)
				}
				if d.evalBusy {
					out.TextColor(t.TextMuted)
				}
			})
		})
		dialogButtons(c, func() {
			if textButton(c, "关闭", buttonPlain, nil).Clicked() {
				d.evalOpen = false
			}
			if textButton(c, "添加为监视", buttonPlain, eyeIcon).Clicked() {
				a.addWatchFromDialog()
			}
			calc := textButton(c, map[bool]string{true: "计算中…", false: "计算"}[d.evalBusy], buttonAccent, nil)
			if d.evalBusy {
				calc.Disabled(true)
			}
			if calc.Clicked() {
				a.runEvaluateExpression()
			}
		})
	})
}

func (a *app) conditionDialog(c *ui.Context) {
	d := &a.dlg
	if !d.condOpen {
		return
	}
	a.dialog(c, &d.condOpen, 440, 0, func() {
		dialogTitle(c, breakpointIcon, "断点条件", 1.15)
		var in ui.Element
		ui.Field(c, d.condTitle+" 的条件表达式，留空表示无条件命中", func() {
			in = a.dialogField(c, &d.condExpr, "例如：i > 10 && name != null", "断点条件")
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.condOpen = false
			}
			if textButton(c, "保存", buttonAccent, nil).Clicked() || in.Submitted() {
				a.saveBreakpointCondition()
			}
		})
	})
}

func (a *app) inputDialog(c *ui.Context) {
	d := &a.dlg
	if !d.inputOpen {
		return
	}
	a.dialog(c, &d.inputOpen, 420, 0, func() {
		dialogTitle(c, fileIcon, d.inputTitle, 1.15)
		var in ui.Element
		ui.Field(c, d.inputLabel, func() {
			in = a.dialogField(c, &d.inputValue, "com.test.Main", d.inputLabel)
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.inputOpen = false
			}
			ok := textButton(c, "确定", buttonAccent, nil)
			if strings.TrimSpace(d.inputValue) == "" {
				ok.Disabled(true)
			}
			if ok.Clicked() || (in.Submitted() && strings.TrimSpace(d.inputValue) != "") {
				a.submitInput()
			}
		})
	})
}

func (a *app) errorDialog(c *ui.Context) {
	d := &a.dlg
	if !d.errorOpen {
		return
	}
	t := c.Theme()
	a.dialog(c, &d.errorOpen, 560, t.Space(80), func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			badge(c, trashIcon, t.Rem(1.8))
			ui.Text(c, "错误").FontSize(t.Rem(1.15)).Bold().Grow(1)
			if textButton(c, "关闭", buttonPlain, nil).Clicked() {
				d.errorOpen = false
			}
		})
		ui.TextArea(c, &d.errorText).ReadOnly(true).Grow(1).FillWidth().MinHeight(0).Label("错误详情")
	})
}
