package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const appVersion = "1.0"
const mygoVersion = "0.5.5"

func (a *app) mainView(c *ui.Context) {
	s := a.sess
	if s == nil {
		return
	}
	s.runPending()
	if s.notice != "" {
		c.AddToast(ui.Toast{Title: s.notice, Type: s.noticeKind})
		s.notice, s.noticeKind = "", ""
	}
	a.svc = c.Services()
	window(c).Children(func() {
		ui.SplitVertical(c, &s.consoleSplit, func() {
			ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				a.sideRail(c, s)
				ui.Split(c, &s.treeSplit, func() {
					a.sideToolsPane(c, s)
				}, func() {
					a.editorArea(c, s)
				}).Grow(1)
			})
		}, func() {
			a.consolePane(c, s)
		}).Grow(1)
		a.statusBar(c, s)
		a.toastViewport(c)
	})
	a.mainShortcuts(c, s)
	a.mainDialogs(c)
}

func (a *app) mainShortcuts(c *ui.Context, s *session) {
	c.OnShortcut(ui.Ctrl, ui.KeyS, func() { a.saveActiveFile() })
	c.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyF, func() { s.showFindPane() })
	c.OnShortcut(ui.Ctrl, ui.KeyB, func() {
		if s.ed != nil {
			s.onDefinition(s.ed.Cursor())
		}
	})
	c.OnShortcut(ui.Shift, ui.KeyF10, func() { a.runProject() })
	c.OnShortcut(ui.Ctrl, ui.KeyF5, func() { a.buildProject() })
	c.OnShortcut(ui.Shift, ui.KeyF9, func() { a.debugProject() })
	c.OnShortcut(0, ui.KeyF9, func() { a.debugResume() })
	c.OnShortcut(0, ui.KeyF8, func() { a.debugStepOver() })
	c.OnShortcut(0, ui.KeyF7, func() { a.debugStepInto() })
	c.OnShortcut(ui.Shift, ui.KeyF8, func() { a.debugStepOut() })
	c.OnShortcut(ui.Ctrl, ui.KeyF2, func() {
		if s := a.sess; s != nil && s.dbg != nil {
			s.dbg.stop()
		}
	})
	c.OnShortcut(ui.Alt, ui.KeyF9, func() { a.debugRunToCursor() })
	c.OnShortcut(ui.Alt, ui.KeyF8, func() { a.openEvaluateDialog() })
	c.OnShortcut(0, ui.KeyF5, func() { s.refreshTree() })
	c.OnShortcut(ui.Ctrl, ui.KeyN, func() {
		a.openInputDialog(inputCreateFile, "创建新的Java文件", "类名称（含包名，如 com.test.Main）")
	})
}

func (a *app) editorArea(c *ui.Context, s *session) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		a.editorTabs(c, s)
		a.editorBody(c, s)
	})
}

func (a *app) editorTabs(c *ui.Context, s *session) {
	t := c.Theme()
	bar := ui.Row(c).FillWidth().Shrink(0).Gap(t.Space(0.5)).Padding(t.Space(0.5), t.Space(1.5), 0, t.Space(1.5)).
		AlignItems(ui.Center)
	bar.Children(func() {
		if len(s.tabs) == 0 {
			ui.Box(c).Grow(1)
			return
		}
		ui.ScrollHorizontal(c).Grow(1).Gap(t.Space(0.5)).AlignItems(ui.Center).Children(func() {
			for i, tb := range s.tabs {
				on := i == s.active
				tab := ui.ButtonBase(c).Key(tb.path).Shrink(0).Padding(t.Space(1), t.Space(2.5)).
					Gap(t.Space(1.25)).AlignItems(ui.Center).Radius(t.Radius, t.Radius, 0, 0).
					Cursor(ui.CursorPointer).Label(tb.name)
				if on {
					tab.Background(t.Surface).TextColor(t.Text)
				} else {
					tab.TextColor(t.TextMuted)
					if tab.Hovered() {
						tab.Background(t.Background.Mix(t.Text, 0.06))
					}
				}
				tab.Children(func() {
					if on {
						ui.Box(c).Absolute().Top(0).Left(t.Space(2)).Right(t.Space(2)).Height(2).
							Background(t.Accent).Radius(1, 1, 0, 0)
					}
					ui.Icon(c, map[bool]*ui.SVG{true: classIcon, false: fileIcon}[tb.readOnly]).
						FontSize(iconSize(t, 0.9))
					ui.Text(c, tb.name).SingleLine().FontSize(t.Rem(0.85))
					if tb.ed != nil && tb.ed.Buffer != nil && tb.ed.Buffer.Modified() {
						ui.Text(c, "●").FontSize(t.Rem(0.65)).TextColor(t.Accent)
					}
					if tabCloseButton(c, tb.name).Clicked() {
						s.later(func() { s.closeTab(i) })
					}
				})
				tab.ContextMenu(func(m *ui.Menu) {
					if m.Item("关闭").Chosen() {
						s.later(func() { s.closeTab(i) })
					}
					if m.Item("关闭其他").Chosen() {
						s.later(func() {
							for j := range slices.Backward(s.tabs) {
								if j != i {
									s.closeTab(j)
								}
							}
						})
					}
					if m.Item("关闭全部").Chosen() {
						s.later(func() {
							for j := range slices.Backward(s.tabs) {
								s.closeTab(j)
							}
						})
					}
					m.Separator()
					if m.Item("在文件管理器中显示").Chosen() {
						mygo.Shell.ShowItemInFolder(tb.path)
					}
					if m.Item("复制路径").Chosen() {
						a.copyToClipboard(tb.path)
					}
				})
				if tab.Clicked() {
					s.switchTab(i)
				}
			}
		})
	})
}

func (a *app) editorBody(c *ui.Context, s *session) {
	t := c.Theme()
	tb := s.activeTab()
	if tb == nil || tb.ed == nil {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			emptyState(c, fileIcon, "没有打开的文件", "从左侧文件树选择一个 Java 源文件开始编辑")
		})
		return
	}
	s.applyDebugMarks(t, tb)
	editor.View(c, tb.ed).Grow(1).FillWidth().Label("代码编辑器")
	if !tb.readOnly && s.settings.AutoSave && tb.ed.Buffer.Modified() {
		a.saveActiveFile()
	}
}

func (a *app) consolePane(c *ui.Context, s *session) {
	t := c.Theme()
	bar, foot := t.Space(10), t.Space(9)
	color := t.TextMuted
	switch {
	case s.running:
		color = t.Success
	case s.exitCode == 0:
		color = t.Success
	case s.exitCode > 0:
		color = t.Danger
	}
	panel(c, func() {
		scrollPane(c, bar, foot+t.Space(2.5), func() {
			paneHeader(c, consoleIcon, "控制台", func() {
				pill(c, s.status(), color)
				if iconButton(c, "清除控制台", trashIcon).Clicked() {
					s.clearConsole()
				}
				follow := s.consoleList.FollowEnd
				fb := iconButton(c, "自动跟随", followIcon)
				if follow {
					fb.Background(activeOf(t)).TextColor(t.Accent)
				}
				if fb.Clicked() {
					s.consoleList.FollowEnd = !follow
				}
			})
		}, func() {
			s.consoleList.Selected = &s.consoleSel
			ui.List(c, &s.consoleList, len(s.console)).Grow(1).FillWidth().PaddingX(t.Space(1)).Rows(func(r ui.ListRow) {
				e := s.console[r.Index]
				text := ui.Text(r.Context, e.text).FontSize(t.Rem(0.85)).SingleLine()
				if a.mono != "" {
					text.Font(a.mono)
				}
				switch e.kind {
				case consoleErr:
					text.TextColor(t.Danger)
				case consoleInfo:
					text.TextColor(t.TextMuted)
				case consoleInput:
					text.TextColor(t.Accent)
				default:
					text.TextColor(t.Text)
				}
				if e.loc != nil {
					text.Underline()
				}
			})
			if s.consoleSel >= 0 && s.consoleSel < len(s.console) {
				e := s.console[s.consoleSel]
				s.consoleSel = -1
				if e.loc != nil {
					file, line, col := e.loc.file, e.loc.line, e.loc.col
					s.later(func() { a.openFileAt(file, line, col) })
				}
			}
		})
		ui.Row(c).Absolute().Bottom(t.Space(1.5)).Left(t.Space(1.5)).Right(t.Space(1.5)).Height(foot).
			Padding(t.Space(0.75), t.Space(1.5)).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &s.consoleInput).Placeholder("输入后回车发送给进程").
				Grow(1).Disabled(!s.running).Label("进程输入")
			if in.Submitted() && s.running {
				line := s.consoleInput
				if line != "" {
					s.engine.send(line + "\n")
					s.appendOutput(line+"\n", consoleInput)
				}
				s.consoleInput = ""
			}
		})
	})
}

func (a *app) statusBar(c *ui.Context, s *session) {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Shrink(0).Gap(t.Space(2.5)).Padding(t.Space(1), t.Space(3)).
		AlignItems(ui.Center).Background(t.Surface)
	row.Children(func() {
		if s.ed != nil && s.ed.Buffer != nil {
			pos := s.ed.CursorPos()
			statusText(c, "行 "+strconv.Itoa(pos.Line+1)+", 列 "+strconv.Itoa(pos.Col+1))
			statusText(c, fileLanguage(s.currentPath))
		}
		ui.Box(c).Grow(1)
		if s.git.isRepo && s.git.branch != "" {
			ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, branchIcon).FontSize(iconSize(t, 0.8)).TextColor(t.TextMuted)
				statusText(c, s.git.branch)
			})
		}
		statusText(c, s.toolLabel(s.buildToolName()))
		if tb := s.activeTab(); tb != nil && tb.readOnly {
			statusText(c, "只读")
		}
		if s.dbg != nil {
			statusTextColored(c, s.dbg.statusText(), t.Warning)
		} else if s.running {
			pill(c, "运行中", t.Success)
		}
	})
}

func statusText(c *ui.Context, s string) {
	t := c.Theme()
	ui.Text(c, s).FontSize(t.Rem(0.78)).TextColor(t.TextMuted).SingleLine()
}

func statusTextColored(c *ui.Context, s string, color ui.Color) {
	t := c.Theme()
	ui.Text(c, s).FontSize(t.Rem(0.78)).TextColor(color).SingleLine()
}

func fileLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".java":
		return "Java"
	case ".class":
		return "字节码"
	case ".json":
		return "JSON"
	case ".md":
		return "Markdown"
	case ".xml", ".gradle", ".kts":
		return "XML"
	default:
		return "文本"
	}
}

func (a *app) openFile(path string) {
	s := a.sess
	if s == nil {
		return
	}
	name := filepath.Base(path)
	if strings.EqualFold(filepath.Ext(path), ".class") {
		s.openTab(path, name, true)
		go func() {
			res := decompileClass(path, a.cfg.Settings.JdkHome)
			a.update(func() {
				s.setEditor(res.Output, name, true)
				if res.ExitCode != 0 {
					s.appendError(res.Output)
				}
			})
		}()
		return
	}
	if s.findTab(path) != nil {
		s.activateTab(path)
		s.rememberFile(path)
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		a.showError("无法打开文件：" + err.Error())
		return
	}
	s.openTab(path, name, false)
	s.setEditor(string(b), name, false)
	s.rememberFile(path)
}

func (a *app) openFileAt(path string, line, col int) {
	s := a.sess
	if s == nil {
		return
	}
	if tb := s.activeTab(); tb == nil || filepath.Clean(tb.path) != filepath.Clean(path) {
		a.openFile(path)
	}
	if s.ed != nil && line > 0 {
		s.ed.GoTo(editor.Pos{Line: max(line-1, 0), Col: max(col-1, 0)})
	}
}

func (a *app) saveActiveFile() {
	s := a.sess
	if s == nil {
		return
	}
	if err := s.saveCurrentFile(); err != nil {
		a.showError("保存文件失败：" + err.Error())
	}
}

func (a *app) cleanOutput() {
	s := a.sess
	if s == nil {
		return
	}
	s.appendOutput("正在清理输出目录...\n", consoleInfo)
	go func() {
		err := os.RemoveAll(filepath.Join(s.path, "out"))
		a.update(func() {
			if err != nil {
				s.appendError("清理失败：" + err.Error())
				return
			}
			s.appendOutput("输出目录已清理\n", consoleInfo)
			s.refreshTree()
		})
	}()
}

func (a *app) packageJar() {
	s := a.sess
	if s == nil {
		return
	}
	st := s.settings.normalized()
	s.appendOutput("正在构建 JAR...\n", consoleInfo)
	go func() {
		res := jarPackage(s.path, s.activeRunConfig().mainClassOrDefault(s.mainClass), st.JdkHome)
		a.update(func() {
			if res.ExitCode != 0 {
				s.appendError(res.Output)
				a.showError(res.Output)
				return
			}
			s.appendOutput(res.Output+"\n", consoleInfo)
			s.refreshTree()
			notifyBuild("构建完成", "已生成可运行 JAR")
		})
	}()
}

func (a *app) buildProject() {
	s := a.sess
	if s == nil {
		return
	}
	tool := s.buildToolName()
	s.appendOutput("正在使用 "+s.toolLabel(tool)+" 构建项目...\n", consoleInfo)
	go func() {
		res := s.toolBuild(tool)
		a.update(func() {
			if res.Output != "" {
				s.appendOutput(res.Output, consoleOut)
			}
			if res.ExitCode != 0 {
				s.buildErrors = s.collectBuildErrors(res.Output)
				s.appendError("构建失败\n")
				s.refreshProblems()
				notifyBuild("构建失败", "查看控制台了解详情")
				return
			}
			s.buildErrors = s.collectBuildErrors(res.Output)
			s.appendOutput("构建成功！\n", consoleInfo)
			s.refreshTree()
			s.refreshProblems()
			s.indexSymbols()
		})
	}()
}

func (a *app) runProject() {
	s := a.sess
	if s == nil {
		return
	}
	if s.running {
		s.running = false
		s.exitCode = -1
		s.engine.stop()
		s.appendOutput("进程已被停止\n", consoleInfo)
		return
	}
	rc := s.activeRunConfig()
	main := rc.mainClassOrDefault(s.mainClass)
	if main == "" {
		return
	}
	st := s.settings.normalized()
	tool := s.buildToolName()
	s.clearConsole()
	s.appendOutput("正在使用 "+s.toolLabel(tool)+" 构建项目...\n", consoleInfo)
	go func() {
		build := s.toolBuild(tool)
		if build.Output != "" {
			a.update(func() { s.appendOutput(build.Output, consoleOut) })
		}
		if build.ExitCode != 0 {
			a.update(func() {
				s.buildErrors = s.collectBuildErrors(build.Output)
				s.appendError("编译失败：\n")
				s.refreshProblems()
				notifyBuild("构建失败", "查看控制台了解详情")
			})
			return
		}
		a.update(func() {
			s.refreshTree()
			s.running = true
			s.exitCode = -1
			s.appendOutput("编译完成，程序已启动："+main+"\n", consoleInfo)
			if len(rc.ProgramArgs) > 0 {
				s.appendOutput("程序参数："+strings.Join(rc.ProgramArgs, " ")+"\n", consoleInfo)
			}
		})
		started := time.Now()
		res := s.engine.run(s.path, runOptions{
			javaCommand: s.javaCommand, mainClass: main, classpath: s.classpath(),
			vmArgs: rc.vmArguments(), programArgs: rc.programArguments(),
			env: rc.environment(), workDir: rc.WorkDir, jdkHome: st.JdkHome,
		}, func(chunk string) {
			a.update(func() { s.appendOutput(chunk, consoleOut) })
		})
		a.update(func() {
			s.running = false
			s.exitCode = res.ExitCode
			s.appendOutput(fmt.Sprintf("\n进程已结束，退出代码 %d，运行 %s\n", res.ExitCode, time.Since(started).Round(time.Millisecond)), consoleInfo)
		})
	}()
}

func (a *app) debugProject() {
	s := a.sess
	if s == nil {
		return
	}
	if s.dbg != nil {
		s.dbg.stop()
		return
	}
	if s.running {
		s.running = false
		s.exitCode = -1
		s.engine.stop()
		s.appendOutput("进程已被停止\n", consoleInfo)
		return
	}
	rc := s.activeRunConfig()
	main := rc.mainClassOrDefault(s.mainClass)
	if main == "" {
		a.showError("尚未配置主类，请先在「运行 → 项目配置」中设置")
		return
	}
	st := s.settings.normalized()
	tool := s.buildToolName()
	sources := javaSourceFiles(s.path)
	s.clearConsole()
	s.appendOutput("正在使用 "+s.toolLabel(tool)+" 构建项目...\n", consoleInfo)
	go func() {
		build := s.toolBuild(tool)
		if build.Output != "" {
			a.update(func() { s.appendOutput(build.Output, consoleOut) })
		}
		if build.ExitCode != 0 {
			a.update(func() {
				s.buildErrors = s.collectBuildErrors(build.Output)
				s.appendError("编译失败：\n")
				s.refreshProblems()
				notifyBuild("构建失败", "查看控制台了解详情")
			})
			return
		}
		a.update(func() {
			s.refreshTree()
			s.refreshProblems()
			s.sideTab = sideDebug
			s.startDebug(main, rc, st, sources)
		})
	}()
}

func (a *app) debugResume() {
	if s := a.sess; s != nil && s.dbg != nil {
		s.dbg.resume()
	}
}

func (a *app) debugStepInto() {
	if s := a.sess; s != nil && s.dbg != nil {
		s.dbg.step(stepInto)
	}
}

func (a *app) debugStepOver() {
	if s := a.sess; s != nil && s.dbg != nil {
		s.dbg.step(stepOver)
	}
}

func (a *app) debugStepOut() {
	if s := a.sess; s != nil && s.dbg != nil {
		s.dbg.step(stepOut)
	}
}

func (a *app) debugPause() {
	if s := a.sess; s != nil && s.dbg != nil {
		s.dbg.pause()
	}
}

func (a *app) debugRunToCursor() {
	s := a.sess
	if s == nil || s.dbg == nil || s.ed == nil {
		return
	}
	tb := s.activeTab()
	if tb == nil {
		return
	}
	s.dbg.runToCursor(tb.path, s.ed.CursorPos().Line+1)
}

func (a *app) debugAddWatch() {
	s := a.sess
	if s == nil || s.dbg == nil {
		a.showError("请先开始调试会话")
		return
	}
	if s.ed != nil {
		if word := s.wordUnderCaret(); word != "" {
			s.dbg.addWatch(word)
			return
		}
	}
	a.openWatchDialog()
}

func (a *app) toastViewport(c *ui.Context) {
	t := c.Theme()
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Absolute().Bottom(t.Space(14)).Right(t.Space(3)).Padding(t.Space(2)).
			AlignItems(ui.End).Gap(t.Space(1)).PassThrough()
		for _, ts := range toasts {
			toast := ui.ToastBase(c, ts)
			root := toast.Root.Padding(t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).
				AlignItems(ui.Center).Background(t.Surface).Border(1, t.Border).Radius(t.Radius).
				Shadow(0, t.Space(1.5), t.Space(5), -t.Space(1), shadowOf(t))
			root.Children(func() {
				color, icon := t.Success, buildIcon
				if ts.Type == "error" {
					color, icon = t.Danger, problemIcon
				}
				ui.Icon(c, icon).FontSize(iconSize(t, 0.95)).TextColor(color)
				ui.Text(c, ts.Title).FontSize(t.Rem(0.85)).SingleLine()
				toast.CloseButton().Label("关闭提示").Children(func() {
					ui.Icon(c, closeIcon).FontSize(iconSize(t, 0.85))
				})
			})
		}
	})
}
