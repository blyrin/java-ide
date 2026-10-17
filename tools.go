package main

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	sideFiles = iota
	sideStructure
	sideFind
	sideGit
	sideProblems
	sideDebug
)

var sideTools = []struct {
	name  string
	icon  *ui.SVG
	tab   int
	badge func(s *session) int
}{
	{"文件", folderIcon, sideFiles, nil},
	{"结构", structureIcon, sideStructure, nil},
	{"查找", searchIcon, sideFind, nil},
	{"版本控制", gitIcon, sideGit, func(s *session) int { return len(s.git.files) }},
	{"问题", problemIcon, sideProblems, func(s *session) int { return len(s.allProblems()) }},
	{"调试", threadIcon, sideDebug, func(s *session) int {
		if s.dbg != nil {
			return 1
		}
		return 0
	}},
}

func (a *app) sideRail(c *ui.Context, s *session) {
	t := c.Theme()
	rail := ui.Column(c).Fill().Width(t.Space(11)).Shrink(0).Background(t.Surface).
		PaddingY(t.Space(1.5)).Gap(t.Space(1)).AlignItems(ui.Center)
	rail.Children(func() {
		for _, tool := range sideTools {
			count := 0
			if tool.badge != nil {
				count = tool.badge(s)
			}
			a.railButton(c, s, tool.name, tool.icon, tool.tab, count)
		}
	})
}

func (a *app) railButton(c *ui.Context, s *session, name string, icon *ui.SVG, tab, count int) {
	t := c.Theme()
	on := s.sideTab == tab
	b := ui.ButtonBase(c).Key(name).Size(t.Space(9), t.Space(9)).Center().Radius(t.Radius).
		Cursor(ui.CursorPointer).Label(name).Tooltip(name)
	if on {
		b.Background(activeOf(t)).TextColor(t.Accent)
	} else {
		b.TextColor(t.TextMuted)
		if b.Hovered() {
			b.Background(hoverOf(t)).TextColor(t.Text)
		} else {
			b.Background(ui.Transparent)
		}
	}
	b.Children(func() {
		ui.Icon(c, icon).FontSize(iconSize(t, 1.15))
		if count > 0 && !on {
			ui.Box(c).Absolute().Top(t.Space(0.75)).Right(t.Space(0.75)).
				Size(t.Space(1.75), t.Space(1.75)).Background(t.Danger).Radius(t.Space(1))
		}
	})
	if b.Clicked() {
		s.sideTab = tab
	}
}

func (a *app) sideToolsPane(c *ui.Context, s *session) {
	p := ui.Box(c).Fill().Background(c.Theme().Surface)
	p.Children(func() {
		body := ui.Box(c).Fill()
		body.Children(func() {
			switch s.sideTab {
			case sideStructure:
				a.structurePane(c, s)
			case sideFind:
				s.findPane(c)
			case sideGit:
				s.gitPane(c)
			case sideProblems:
				a.problemsPane(c, s)
			case sideDebug:
				a.debugPane(c, s)
			default:
				a.fileTreePane(c, s)
			}
		})
	})
}

func (a *app) fileTreePane(c *ui.Context, s *session) {
	t := c.Theme()
	bar := t.Space(4)
	box := ui.Box(c).Fill()
	box.ContextMenu(func(m *ui.Menu) {
		if m.Item("创建源文件").Chosen() {
			a.openInputDialog(inputCreateFile, "创建新的Java文件", "类名称（含包名，如 com.test.Main）")
		}
		if m.Item("新建目录").Chosen() {
			a.openInputDialog(inputCreateDir, "新建目录", "目录名称（相对于项目根目录）")
		}
		m.Separator()
		if m.Item("刷新").Chosen() {
			s.refreshTree()
		}
		if m.Item("展开全部").Chosen() {
			a.expandTree(s, s.path)
		}
		if m.Item("折叠全部").Chosen() {
			for _, b := range s.open {
				*b = false
			}
		}
		m.Separator()
		if m.Item("在文件管理器中显示").Chosen() {
			mygo.Shell.ShowItemInFolder(s.path)
		}
	})
	box.Children(func() {
		scrollPane(c, bar, t.Space(2), func() {
			paneHeader(c, folderIcon, s.name, func() {
				if iconButton(c, "刷新文件树", refreshIcon).Clicked() {
					s.refreshTree()
				}
			})
		}, func() {
			ui.Tree(c, func() { a.treeNodes(c, s, s.root) }).PaddingX(t.Space(1))
		})
	})
}

func treeIcon(n *treeNode) *ui.SVG {
	if n.dir {
		return folderIcon
	}
	if strings.EqualFold(filepath.Ext(n.name), ".java") {
		return classIcon
	}
	return fileIcon
}

func treeIconColor(c *ui.Context, n *treeNode) ui.Color {
	t := c.Theme()
	if !n.dir && strings.EqualFold(filepath.Ext(n.name), ".java") {
		return t.Accent
	}
	return t.TextMuted
}

func (a *app) expandTree(s *session, dir string) {
	for _, n := range treeLevel(dir, s.showHidden) {
		if !n.dir {
			continue
		}
		*s.openFlag(n.path) = true
		a.expandTree(s, n.path)
	}
}

func (a *app) treeNodes(c *ui.Context, s *session, nodes []*treeNode) {
	t := c.Theme()
	for _, n := range nodes {
		var open *bool
		var children func()
		if n.dir {
			open = s.openFlag(n.path)
			children = func() { a.treeNodes(c, s, treeLevel(n.path, s.showHidden)) }
		}
		item := ui.TreeItem(c.Key(n.path), "", open, children).Label(n.name)
		item.Children(func() {
			ui.Icon(c, treeIcon(n)).FontSize(iconSize(t, 0.85)).TextColor(treeIconColor(c, n))
			ui.Text(c, n.name).SingleLine().Ellipsis("…")
		})
		if item.Clicked() {
			if n.dir {
				*open = !*open
			} else {
				a.openFile(n.path)
			}
		}
		if !n.dir {
			path := n.path
			item.ContextMenu(func(m *ui.Menu) {
				if m.Item("打开").Chosen() {
					a.openFile(path)
				}
				if m.Item("重命名").Chosen() {
					a.openRenameDialog(path)
				}
				if m.Item("复制路径").Chosen() {
					a.copyToClipboard(path)
				}
				m.Separator()
				if m.Item("在文件管理器中显示").Chosen() {
					mygo.Shell.ShowItemInFolder(path)
				}
				m.Separator()
				if m.Item("删除").Chosen() {
					a.confirmDeleteFile(path)
				}
			})
		}
	}
}

func (a *app) structurePane(c *ui.Context, s *session) {
	t := c.Theme()
	scrollContent(c, t.Space(2), func() {
		if s.ed != nil && s.ed.Buffer != nil && s.structureVersion != s.ed.Buffer.Version() {
			s.parseStructure()
		}
		if len(s.structure) == 0 {
			emptyState(c, structureIcon, "没有结构", "打开一个 Java 文件查看其大纲")
			return
		}
		ui.Outline(c, &s.outline, s.structure, func(m *javaMember) []*javaMember { return m.children },
			func(m *javaMember) {
				row := ui.Row(c).FillWidth().Gap(t.Space(1.5)).Padding(t.Space(0.5), t.Space(1)).
					AlignItems(ui.Center).Radius(t.Radius)
				row.Children(func() {
					ui.Icon(c, memberIcon(m.kind)).FontSize(iconSize(t, 0.85)).TextColor(memberColor(c, m.kind))
					ui.Text(c, m.name).FontSize(t.Rem(0.9)).SingleLine().Ellipsis("…")
					if m.kind == javaMethod || m.kind == javaField {
						ui.Text(c, m.detail).FontSize(t.Rem(0.75)).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
					}
				})
				if row.Hovered() {
					row.Background(hoverOf(t))
				}
				if row.Clicked() {
					s.goToLine(m.line)
				}
			}).FillWidth()
	})
}

func memberIcon(k javaKind) *ui.SVG {
	switch k {
	case javaClass:
		return classIcon
	case javaInterface:
		return interfaceIcon
	case javaEnum, javaRecord:
		return enumIcon
	case javaAnnotationType:
		return interfaceIcon
	case javaMethod, javaConstructor:
		return methodIcon
	case javaField, javaEnumConst:
		return fieldIcon
	default:
		return fileIcon
	}
}

func memberColor(c *ui.Context, k javaKind) ui.Color {
	t := c.Theme()
	switch k {
	case javaMethod, javaConstructor:
		return t.Accent
	case javaField, javaEnumConst:
		return t.Success
	default:
		return t.TextMuted
	}
}

func (a *app) problemsPane(c *ui.Context, s *session) {
	t := c.Theme()
	bar := t.Space(4)
	problems := s.allProblems()
	scrollPane(c, bar, t.Space(2), func() {
		paneHeader(c, problemIcon, "问题", func() {
			if len(problems) > 0 {
				ui.Text(c, strconv.Itoa(len(problems))).FontSize(t.Rem(0.78)).TextColor(t.TextMuted)
			}
			if iconButton(c, "重新编译", buildIcon).Clicked() {
				a.buildProject()
			}
		})
	}, func() {
		if len(problems) == 0 {
			emptyState(c, checkIcon, "没有发现问题", "编译错误、警告与 TODO 会显示在这里")
			return
		}
		s.problemsList.Selected = &s.problemsSel
		s.problemsList.Label = func(i int) string { return problems[i].msg }
		ui.List(c, &s.problemsList, len(problems)).Grow(1).FillWidth().Gap(t.Space(0.5)).
			Rows(func(r ui.ListRow) {
				e := problems[r.Index]
				row := ui.Column(r.Context).FillWidth().Gap(0).Padding(t.Space(0.5), t.Space(1))
				if r.Selected() || r.ListFocused() {
					row.Background(t.Background.Mix(t.Accent, 0.16))
				}
				row.Children(func() {
					ui.Row(r.Context).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
						ui.Icon(r.Context, problemIcon).FontSize(iconSize(t, 0.8)).
							TextColor(map[bool]ui.Color{true: t.Warning, false: t.Danger}[e.warn])
						ui.Text(r.Context, e.msg).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
					})
					ui.Text(r.Context, relativeTo(s.path, e.file)+":"+strconv.Itoa(e.line)).
						FontSize(t.Rem(0.75)).TextColor(t.TextMuted).SingleLine()
				})
			})
		if s.problemsSel >= 0 && s.problemsSel < len(problems) {
			e := problems[s.problemsSel]
			s.problemsSel = -1
			file, line, col := e.file, e.line, e.col
			s.later(func() { a.openFileAt(file, line, col) })
		}
	})
}

func relativeTo(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Base(file))
}

func (s *session) parseStructure() {
	text := s.editorText()
	version := s.ed.Buffer.Version()
	app := s.app
	go func() {
		members := parseJavaStructure(text)
		app.update(func() {
			if s.ed == nil || s.ed.Buffer == nil || s.ed.Buffer.Version() != version {
				return
			}
			s.structure = members
			s.structureVersion = version
		})
	}()
}

func (s *session) goToLine(line int) {
	if s.ed == nil {
		return
	}
	s.ed.GoTo(editor.Pos{Line: line, Col: 0})
}

func (a *app) copyToClipboard(text string) {
	a.svc.WriteClipboard(text)
}
