package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type gitFile struct {
	path   string
	status string
	staged bool
}

type gitCommit struct {
	hash    string
	author  string
	date    string
	subject string
}

type gitState struct {
	isRepo    bool
	branch    string
	files     []gitFile
	sel       int
	commitMsg string
	busy      bool
	note      string
	log       []gitCommit
	diff      *editor.DiffState
	diffFile  string
	loading   bool
	list      ui.ListState
	logList   ui.ListState
	logSel    int
	checked   bool
}

func (a *app) gitCommand(s *session, args ...string) (string, error) {
	name := strings.TrimSpace(a.cfg.Settings.GitCommand)
	if name == "" {
		name = "git"
	}
	cmd := newCommand(name, args...)
	cmd.Dir = s.path
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f gitFile) statusLabel() string {
	switch strings.TrimSpace(f.status) {
	case "M", "MM":
		return "已修改"
	case "A":
		return "新增"
	case "D":
		return "已删除"
	case "R", "C":
		return "重命名"
	case "??":
		return "未跟踪"
	case "UU", "AA", "DD":
		return "冲突"
	default:
		return strings.TrimSpace(f.status)
	}
}

func gitInitRepo(root, gitCommand string) error {
	cmd := newCommand(gitTool(gitCommand), "init")
	cmd.Dir = root
	return cmd.Run()
}

func (s *session) gitRefresh() {
	g := &s.git
	if g.checked && !g.isRepo {
		return
	}
	g.loading = true
	app := s.app
	go func() {
		out, err := app.gitCommand(s, "rev-parse", "--is-inside-work-tree")
		if err != nil || strings.TrimSpace(out) != "true" {
			app.update(func() {
				g.isRepo, g.checked, g.loading = false, true, false
				g.note = "当前目录不是 Git 仓库"
			})
			return
		}
		branch, _ := app.gitCommand(s, "rev-parse", "--abbrev-ref", "HEAD")
		status, _ := app.gitCommand(s, "status", "--porcelain")
		logOut, _ := app.gitCommand(s, "log", "--pretty=%h%x09%an%x09%ad%x09%s", "--date=short", "-20")
		files := parseGitStatus(status)
		logs := parseGitLog(logOut)
		app.update(func() {
			g.isRepo, g.checked, g.loading = true, true, false
			g.branch = strings.TrimSpace(branch)
			g.files = files
			g.log = logs
			if g.sel >= len(files) {
				g.sel = len(files) - 1
			}
			g.note = ""
		})
	}()
}

func parseGitStatus(out string) []gitFile {
	var files []gitFile
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := strings.TrimSpace(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		files = append(files, gitFile{path: path, status: status, staged: status[0] != ' ' && status[0] != '?'})
	}
	return files
}

func parseGitLog(out string) []gitCommit {
	var commits []gitCommit
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, gitCommit{hash: parts[0], author: parts[1], date: parts[2], subject: parts[3]})
	}
	return commits
}

func (s *session) gitShowDiff(f gitFile) {
	g := &s.git
	g.loading = true
	app := s.app
	go func() {
		old := ""
		if !strings.HasPrefix(f.status, "??") {
			out, err := app.gitCommand(s, "show", "HEAD:"+filepath.ToSlash(f.path))
			if err == nil {
				old = out
			}
		}
		newText := ""
		if b, err := os.ReadFile(filepath.Join(s.path, f.path)); err == nil {
			newText = string(b)
		}
		app.update(func() {
			g.diffFile = f.path
			d := &editor.DiffState{
				Old: editor.NewBuffer(old), New: editor.NewBuffer(newText),
				Layout: editor.DiffUnified, ContextLines: 3, Wrap: true,
				Font: ui.Font{Family: s.mono, Size: s.settings.normalized().FontSize},
			}
			g.diff, g.loading = d, false
		})
	}()
}

func (s *session) gitStage(f gitFile) {
	app := s.app
	go func() {
		if _, err := app.gitCommand(s, "add", "--", f.path); err != nil {
			app.update(func() { app.showError("暂存失败：" + err.Error()) })
			return
		}
		s.gitRefresh()
	}()
}

func (s *session) gitUnstage(f gitFile) {
	app := s.app
	go func() {
		if _, err := app.gitCommand(s, "restore", "--staged", "--", f.path); err != nil {
			app.update(func() { app.showError("取消暂存失败：" + err.Error()) })
			return
		}
		s.gitRefresh()
	}()
}

func (s *session) gitCommit() {
	msg := strings.TrimSpace(s.git.commitMsg)
	if msg == "" {
		s.app.showError("请先填写提交信息")
		return
	}
	g := &s.git
	g.busy = true
	app := s.app
	go func() {
		if _, err := app.gitCommand(s, "add", "-A"); err != nil {
			app.update(func() {
				g.busy = false
				app.showError("暂存失败：" + err.Error())
			})
			return
		}
		if _, err := app.gitCommand(s, "commit", "-m", msg); err != nil {
			app.update(func() {
				g.busy = false
				app.showError("提交失败：" + err.Error())
			})
			return
		}
		app.update(func() {
			g.busy, g.commitMsg = false, ""
			s.appendOutput("已提交："+msg+"\n", consoleInfo)
			s.gitRefresh()
		})
	}()
}

func (s *session) gitPush() {
	g := &s.git
	g.busy = true
	app := s.app
	go func() {
		out, err := app.gitCommand(s, "push")
		app.update(func() {
			g.busy = false
			if err != nil {
				s.appendError("推送失败：\n" + out)
				return
			}
			s.appendOutput("已推送到远端\n", consoleInfo)
		})
	}()
}

func (s *session) gitPull() {
	g := &s.git
	g.busy = true
	app := s.app
	go func() {
		out, err := app.gitCommand(s, "pull")
		app.update(func() {
			g.busy = false
			if err != nil {
				s.appendError("拉取失败：\n" + out)
				return
			}
			s.appendOutput("已拉取远端更新\n", consoleInfo)
			s.refreshTree()
			s.gitRefresh()
		})
	}()
}

func (s *session) gitInit() {
	app := s.app
	go func() {
		if _, err := app.gitCommand(s, "init"); err != nil {
			app.update(func() { app.showError("初始化仓库失败：" + err.Error()) })
			return
		}
		s.appendOutput("已初始化 Git 仓库\n", consoleInfo)
		s.gitRefresh()
	}()
}

func (s *session) gitPane(c *ui.Context) {
	t := c.Theme()
	g := &s.git
	bar := t.Space(9)
	ui.Box(c).Fill().Children(func() {
		scrollPane(c, bar, t.Space(2.5), func() {
			paneHeader(c, gitIcon, "版本控制", func() {
				if g.loading {
					ui.Spinner(c)
				}
				if iconButton(c, "刷新", refreshIcon).Clicked() {
					g.checked = false
					s.gitRefresh()
				}
			})
		}, func() {
			ui.Column(c).Gap(t.Space(2)).Padding(t.Space(0.5), t.Space(1.5)).Children(func() {
				if !g.isRepo {
					ui.Column(c).Gap(t.Space(1.5)).PaddingY(t.Space(2)).Children(func() {
						ui.Text(c, g.note).TextColor(t.TextMuted)
						if textButton(c, "初始化 Git 仓库", buttonAccent, gitIcon).Clicked() {
							s.gitInit()
						}
					})
					return
				}
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, branchIcon).FontSize(iconSize(t, 0.9)).TextColor(t.TextMuted)
					ui.Text(c, g.branch).FontSize(t.Rem(0.9)).Bold().SingleLine().Ellipsis("…")
					ui.Box(c).Grow(1)
					if textButton(c, "拉取", buttonPlain, pullIcon).Disabled(g.busy).Clicked() {
						s.gitPull()
					}
					if textButton(c, "推送", buttonPlain, pushIcon).Disabled(g.busy).Clicked() {
						s.gitPush()
					}
				})
				if len(g.files) == 0 {
					ui.Text(c, "没有变更").TextColor(t.TextMuted)
				} else {
					sectionLabel(c, "变更 "+strconv.Itoa(len(g.files)))
					g.list.Selected = &g.sel
					ui.List(c, &g.list, len(g.files)).Grow(0).FillWidth().MaxHeight(t.Rem(14)).Gap(t.Space(0.25)).
						Rows(func(r ui.ListRow) {
							f := g.files[r.Index]
							row := ui.Row(r.Context).FillWidth().Gap(t.Space(1.5)).Padding(t.Space(0.5), t.Space(1)).
								AlignItems(ui.Center).Radius(t.Radius)
							if r.Selected() || r.ListFocused() {
								row.Background(activeOf(t))
							}
							row.Children(func() {
								ui.Text(r.Context, f.statusLabel()).FontSize(t.Rem(0.75)).
									TextColor(map[bool]ui.Color{true: t.Success, false: t.Warning}[f.staged]).Width(t.Space(11)).SingleLine()
								ui.Text(r.Context, f.path).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
								if iconButton(r.Context, "查看变更", diffIcon).Tooltip("查看差异").Clicked() {
									s.gitShowDiff(f)
								}
								if f.staged {
									if iconButton(r.Context, "取消暂存", minusIcon).Tooltip("取消暂存").Clicked() {
										s.gitUnstage(f)
									}
								} else if iconButton(r.Context, "暂存", plusIcon).Tooltip("暂存").Clicked() {
									s.gitStage(f)
								}
							})
						})
				}
				sectionLabel(c, "提交")
				ui.TextArea(c, &g.commitMsg).Placeholder("提交信息").Grow(0).FillWidth().MaxHeight(t.Rem(6)).Label("提交信息")
				ui.Row(c).Gap(t.Space(1.5)).Children(func() {
					commit := textButton(c, "提交", buttonAccent, gitIcon)
					if strings.TrimSpace(g.commitMsg) == "" || g.busy {
						commit.Disabled(true)
					}
					if commit.Clicked() {
						s.gitCommit()
					}
				})
				if g.diff != nil {
					ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, diffIcon).FontSize(iconSize(t, 0.9)).TextColor(t.TextMuted)
						ui.Text(c, g.diffFile).FontSize(t.Rem(0.85)).Bold().SingleLine().Ellipsis("…").Grow(1)
						if iconButton(c, "关闭差异", closeIcon).Tooltip("关闭").Clicked() {
							g.diff, g.diffFile = nil, ""
						}
					})
					editor.DiffView(c, g.diff).Grow(0).FillWidth().MaxHeight(t.Rem(24))
				}
				if len(g.log) > 0 {
					sectionLabel(c, "提交历史")
					g.logList.Selected = &g.logSel
					ui.List(c, &g.logList, len(g.log)).Grow(0).FillWidth().MaxHeight(t.Rem(12)).Gap(t.Space(0.25)).
						Rows(func(r ui.ListRow) {
							cm := g.log[r.Index]
							row := ui.Column(r.Context).FillWidth().Gap(0).Padding(t.Space(0.5), t.Space(1)).
								Radius(t.Radius)
							if r.Selected() || r.ListFocused() {
								row.Background(activeOf(t))
							}
							row.Children(func() {
								ui.Row(r.Context).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
									ui.Text(r.Context, cm.hash).FontSize(t.Rem(0.75)).TextColor(t.Accent).SingleLine()
									ui.Text(r.Context, cm.subject).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
								})
								ui.Text(r.Context, cm.author+" · "+cm.date).FontSize(t.Rem(0.75)).TextColor(t.TextMuted)
							})
						})
				}
			})
		})
	})
}
