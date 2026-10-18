package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
)

func (a *app) mainMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "项目", Submenu: []*mygo.MenuItem{
			{ID: "new-file", Label: "新建源文件", Accelerator: "CmdOrCtrl+N", Click: a.menuAction(func() {
				a.openInputDialog(inputCreateFile, "创建新的Java文件", "类名称（含包名，如 com.test.Main）")
			})},
			{Label: "新建目录", Click: a.menuAction(func() {
				a.openInputDialog(inputCreateDir, "新建目录", "目录名称（相对于项目根目录）")
			})},
			mygo.Separator(),
			{ID: "save", Label: "保存", Accelerator: "CmdOrCtrl+S", Click: a.menuAction(func() { a.saveActiveFile() })},
			{Label: "关闭标签", Click: a.menuAction(func() {
				if s := a.sess; s != nil && s.active >= 0 {
					s.later(func() { s.closeTab(s.active) })
				}
			})},
			mygo.Separator(),
			{Label: "刷新文件树", Accelerator: "F5", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.refreshTree()
				}
			})},
			{ID: "recent", Label: "最近打开", Submenu: []*mygo.MenuItem{}},
			mygo.Separator(),
			{Label: "在文件管理器中显示", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					mygo.Shell.ShowItemInFolder(s.path)
				}
			})},
			{Label: "打开项目目录", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					mygo.Shell.OpenPath(s.path)
				}
			})},
			mygo.Separator(),
			{Label: "关闭项目", Accelerator: "CmdOrCtrl+W", Click: a.menuAction(func() {
				if a.win != nil {
					a.win.Close()
				}
			})},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Label: "撤销", Accelerator: "CmdOrCtrl+Z", Click: a.menuAction(func() { a.runEditorCommand(editor.Undo) })},
			{Label: "重做", Accelerator: "CmdOrCtrl+Shift+Z", Click: a.menuAction(func() { a.runEditorCommand(editor.Redo) })},
			mygo.Separator(),
			{Label: "注释/取消注释", Accelerator: "CmdOrCtrl+/", Click: a.menuAction(func() { a.runEditorCommand(editor.ToggleComment) })},
			{Label: "复制当前行", Click: a.menuAction(func() { a.runEditorCommand(editor.DuplicateLinesDown) })},
			{Label: "删除当前行", Click: a.menuAction(func() { a.runEditorCommand(editor.DeleteLine) })},
			{Label: "转为大写", Click: a.menuAction(func() { a.runEditorCommand(editor.Uppercase) })},
			{Label: "转为小写", Click: a.menuAction(func() { a.runEditorCommand(editor.Lowercase) })},
			mygo.Separator(),
			{Label: "查找", Accelerator: "CmdOrCtrl+F", Click: a.menuAction(func() {
				if s := a.sess; s != nil && s.ed != nil {
					s.ed.Find(s.wordUnderCaret())
				}
			})},
			{Label: "在文件中查找", Accelerator: "CmdOrCtrl+Shift+F", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.showFindPane()
				}
			})},
			mygo.Separator(),
			{Label: "折叠全部", Click: a.menuAction(func() { a.runEditorCommand(editor.FoldAll) })},
			{Label: "展开全部", Click: a.menuAction(func() { a.runEditorCommand(editor.UnfoldAll) })},
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{
			{Label: "文件", Click: a.menuAction(func() { a.showTool(sideFiles) })},
			{Label: "结构", Click: a.menuAction(func() { a.showTool(sideStructure) })},
			{Label: "查找", Click: a.menuAction(func() { a.showTool(sideFind) })},
			{Label: "版本控制", Click: a.menuAction(func() { a.showTool(sideGit) })},
			{Label: "问题", Click: a.menuAction(func() { a.showTool(sideProblems) })},
			{Label: "调试", Click: a.menuAction(func() { a.showTool(sideDebug) })},
			mygo.Separator(),
			{ID: "show-hidden", Label: "显示隐藏文件", Click: a.menuAction(func() { a.toggleSetting("showHidden") })},
			{ID: "line-numbers", Label: "显示行号", Click: a.menuAction(func() { a.toggleSetting("lineNumbers") })},
			{ID: "soft-wrap", Label: "软换行", Click: a.menuAction(func() { a.toggleSetting("softWrap") })},
			{ID: "auto-save", Label: "修改后自动保存", Click: a.menuAction(func() { a.toggleSetting("autoSave") })},
			mygo.Separator(),
			{ID: "theme-system", Type: mygo.MenuItemRadio, Label: "跟随系统", Click: a.menuAction(func() { a.setTheme(themeSystem) })},
			{ID: "theme-light", Type: mygo.MenuItemRadio, Label: "浅色", Click: a.menuAction(func() { a.setTheme(themeLight) })},
			{ID: "theme-dark", Type: mygo.MenuItemRadio, Label: "深色", Click: a.menuAction(func() { a.setTheme(themeDark) })},
		}},
		{Label: "构建", Submenu: []*mygo.MenuItem{
			{Label: "构建项目", Accelerator: "CmdOrCtrl+F5", Click: a.menuAction(func() { a.buildProject() })},
			{Label: "构建 JAR", Click: a.menuAction(func() { a.packageJar() })},
			{Label: "清理输出目录", Click: a.menuAction(func() { a.cleanOutput() })},
		}},
		{Label: "运行", Submenu: []*mygo.MenuItem{
			{ID: "run-main", Label: "主类：未配置", Disabled: true},
			{ID: "run-config", Label: "选择运行配置", Submenu: []*mygo.MenuItem{}},
			mygo.Separator(),
			{Label: "运行", Accelerator: "Shift+F10", Click: a.menuAction(func() { a.runProject() })},
			{Label: "停止", Click: a.menuAction(func() {
				if s := a.sess; s != nil && s.running {
					a.runProject()
				}
			})},
			mygo.Separator(),
			{Label: "编辑运行配置", Click: a.menuAction(func() { a.openRunDialog() })},
			{Label: "新建运行配置", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.addRunConfig()
					a.openRunDialog()
				}
			})},
			{Label: "删除当前运行配置", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.removeRunConfig()
				}
			})},
			mygo.Separator(),
			{Label: "项目配置", Click: a.menuAction(func() { a.openConfigDialog() })},
		}},
		{Label: "调试", Submenu: []*mygo.MenuItem{
			{ID: "debug-start", Label: "开始调试", Accelerator: "Shift+F9", Click: a.menuAction(func() { a.debugProject() })},
			{ID: "debug-stop", Label: "停止调试", Accelerator: "CmdOrCtrl+F2", Click: a.menuAction(func() {
				if s := a.sess; s != nil && s.dbg != nil {
					s.dbg.stop()
				}
			})},
			mygo.Separator(),
			{ID: "debug-resume", Label: "继续", Accelerator: "F9", Click: a.menuAction(func() { a.debugResume() })},
			{ID: "debug-pause", Label: "暂停", Click: a.menuAction(func() { a.debugPause() })},
			{ID: "debug-over", Label: "单步跳过", Accelerator: "F8", Click: a.menuAction(func() { a.debugStepOver() })},
			{ID: "debug-into", Label: "单步进入", Accelerator: "F7", Click: a.menuAction(func() { a.debugStepInto() })},
			{ID: "debug-out", Label: "单步跳出", Accelerator: "Shift+F8", Click: a.menuAction(func() { a.debugStepOut() })},
			{ID: "debug-cursor", Label: "运行至光标", Accelerator: "Alt+F9", Click: a.menuAction(func() { a.debugRunToCursor() })},
			mygo.Separator(),
			{ID: "debug-eval", Label: "计算表达式", Accelerator: "Alt+F8", Click: a.menuAction(func() { a.openEvaluateDialog() })},
			{ID: "debug-watch", Label: "添加监视表达式", Click: a.menuAction(func() { a.debugAddWatch() })},
			mygo.Separator(),
			{Label: "切换断点", Click: a.menuAction(func() { a.toggleBreakpointAtCaret() })},
			{ID: "debug-cond", Label: "编辑断点条件", Click: a.menuAction(func() { a.editBreakpointConditionAtCaret() })},
			{ID: "debug-bp-enable", Label: "启用全部断点", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.enableAllBreakpoints(true)
				}
			})},
			{ID: "debug-bp-disable", Label: "禁用全部断点", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.enableAllBreakpoints(false)
				}
			})},
			{ID: "debug-bp-clear", Label: "删除所有断点", Click: a.menuAction(func() {
				if s := a.sess; s != nil {
					s.clearBreakpoints()
				}
			})},
		}},
		{Label: "工具", Submenu: []*mygo.MenuItem{
			{Label: "设置", Click: a.menuAction(func() { a.openSettingsDialog() })},
		}},
		{Label: "帮助", Submenu: []*mygo.MenuItem{
			{Label: "关于", Click: a.menuAction(func() { a.dlg.aboutOpen = true })},
		}},
	})
}

func (a *app) menuAction(fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) {
		a.update(fn)
	}
}

func (a *app) runEditorCommand(cmd editor.Command) {
	if s := a.sess; s != nil && s.ed != nil {
		s.ed.Run(cmd)
	}
}

func (a *app) showTool(tab int) {
	if s := a.sess; s != nil {
		s.sideTab = tab
	}
}

func (a *app) toggleSetting(which string) {
	st := a.cfg.settings()
	switch which {
	case "theme:" + themeSystem:
		st.Theme = themeSystem
	case "theme:" + themeLight:
		st.Theme = themeLight
	case "theme:" + themeDark:
		st.Theme = themeDark
	case "showHidden":
		st.ShowHidden = !st.ShowHidden
	case "lineNumbers":
		st.LineNumbers = !st.LineNumbers
	case "softWrap":
		st.SoftWrap = !st.SoftWrap
	case "autoSave":
		st.AutoSave = !st.AutoSave
	}
	a.cfg.Settings = st
	if mygo.App.IsReady() {
		mygo.Theme.SetSource(st.themeSource())
	}
	a.mono = st.monoFamily()
	if s := a.sess; s != nil {
		s.applySettings(st)
	}
	a.cfg.save()
}

func (a *app) setTheme(theme string) {
	a.toggleSetting("theme:" + theme)
}

func (a *app) menuSignature() string {
	st := a.cfg.settings()
	recent, run, main := "", "", ""
	debug, breaks := -2, -1
	if s := a.sess; s != nil {
		if len(s.recent) > 0 {
			recent = s.recent[0]
		}
		run = fmt.Sprint(s.activeRun, "/", s.runConfigNames())
		main = s.activeRunConfig().mainClassOrDefault(s.mainClass)
		if s.dbg != nil {
			debug = int(s.dbg.currentState())
		}
		breaks = len(s.breakpoints)
	}
	return fmt.Sprint(st.Theme, "|", st.ShowHidden, "|", st.LineNumbers, "|",
		st.SoftWrap, "|", st.AutoSave, "|", recent, "|", run, "|", main, "|", debug, "|", breaks)
}

func (a *app) refreshMenuState() {
	m := a.menu
	if m == nil {
		return
	}
	sig := a.menuSignature()
	if sig == a.menuSig {
		return
	}
	a.menuSig = sig
	set := func(id string, on bool) {
		if it := m.ItemByID(id); it != nil && it.IsChecked() != on {
			it.SetChecked(on)
		}
	}
	st := a.cfg.settings()
	set("show-hidden", st.ShowHidden)
	set("line-numbers", st.LineNumbers)
	set("soft-wrap", st.SoftWrap)
	set("auto-save", st.AutoSave)
	set("theme-system", st.Theme == themeSystem)
	set("theme-light", st.Theme == themeLight)
	set("theme-dark", st.Theme == themeDark)
	if it := m.ItemByID("run-main"); it != nil {
		main := "未配置主类"
		if s := a.sess; s != nil {
			if mc := s.activeRunConfig().mainClassOrDefault(s.mainClass); mc != "" {
				main = mc
			}
		}
		if label := "主类：" + main; it.Label != label {
			it.SetLabel(label)
		}
	}
	enable := func(id string, on bool) {
		if it := m.ItemByID(id); it != nil && it.IsEnabled() != on {
			it.SetEnabled(on)
		}
	}
	debugPhase, breaks := -2, -1
	if s := a.sess; s != nil {
		if s.dbg != nil {
			debugPhase = int(s.dbg.currentState())
		}
		breaks = len(s.breakpoints)
	}
	stopped := debugPhase == int(debugSuspended)
	running := debugPhase == int(debugRunning) || debugPhase == int(debugLaunching)
	enable("debug-start", debugPhase == -2)
	enable("debug-stop", debugPhase != -2)
	enable("debug-resume", stopped)
	enable("debug-pause", running)
	enable("debug-over", stopped)
	enable("debug-into", stopped)
	enable("debug-out", stopped)
	enable("debug-cursor", stopped)
	enable("debug-eval", stopped)
	enable("debug-watch", stopped)
	enable("debug-cond", breaks > 0)
	enable("debug-bp-enable", breaks > 0)
	enable("debug-bp-disable", breaks > 0)
	enable("debug-bp-clear", breaks > 0)
	if it := m.ItemByID("run-config"); it != nil {
		var items []*mygo.MenuItem
		if s := a.sess; s != nil {
			for i, r := range s.runs {
				idx, rc := i, r
				label := rc.Name
				if mc := rc.mainClassOrDefault(s.mainClass); mc != "" {
					label += "：" + mc
				}
				item := &mygo.MenuItem{
					ID:      fmt.Sprintf("run-config-%d", idx),
					Type:    mygo.MenuItemRadio,
					Label:   label,
					Checked: idx == s.activeRun,
					Click: func(*mygo.MenuItem, *mygo.Window) {
						a.update(func() {
							if ss := a.sess; ss != nil {
								ss.setActiveRunConfig(idx)
							}
						})
					},
				}
				items = append(items, item)
			}
		}
		if len(items) == 0 {
			items = append(items, &mygo.MenuItem{Label: "没有运行配置", Disabled: true})
		}
		it.Submenu = items
		it.SetLabel(it.Label)
	}
	if it := m.ItemByID("recent"); it != nil {
		var items []*mygo.MenuItem
		if s := a.sess; s != nil {
			for _, p := range s.recent {
				path := p
				items = append(items, &mygo.MenuItem{
					Label: filepath.Base(path),
					Click: func(*mygo.MenuItem, *mygo.Window) { a.update(func() { a.openFile(path) }) },
				})
			}
		}
		if len(items) == 0 {
			items = append(items, &mygo.MenuItem{Label: "没有最近文件", Disabled: true})
		}
		it.Submenu = items
		it.SetLabel(it.Label)
	}
}
