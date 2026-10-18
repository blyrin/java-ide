package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var avatarColors = []string{
	"#f7a8c4", "#ffb347", "#00c957", "#a066d3", "#e3cf57",
	"#dda0dd", "#33a1d2", "#2e8b57", "#fce6c9", "#808a87",
}

func (a *app) welcomeView(c *ui.Context) {
	t := c.Theme()
	a.svc = c.Services()
	window(c).Children(func() {
		a.welcomeHeader(c)
		ui.Column(c).Gap(t.Space(2)).Padding(t.Space(2)).Children(func() {
			a.welcomeActions(c)
			a.welcomeList(c)
		})
	})
	a.welcomeDialogs(c)
}

func (a *app) welcomeHeader(c *ui.Context) {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Shrink(0).Gap(t.Space(3)).Padding(t.Space(3), t.Space(3.5)).
		AlignItems(ui.Center).Background(t.Surface)
	row.Children(func() {
		badge(c, javaIcon, t.Rem(2.6))
		ui.Column(c).Shrink(0).Gap(t.Space(0.25)).Children(func() {
			ui.Text(c, "Java IDE").FontSize(t.Rem(1.35)).Bold()
			ui.Text(c, "用 MyGo 原生 UI 构建的 Java 集成开发环境").
				FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
		ui.Box(c).Grow(1)
		if iconButton(c, "关于", infoIcon).Clicked() {
			a.dlg.aboutOpen = true
		}
		if iconButton(c, "设置", gearIcon).Clicked() {
			a.openSettingsDialog()
		}
	})
}

func (a *app) welcomeActions(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Shrink(0).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(t.Rem(40)).Gap(t.Space(2.5)).Children(func() {
			ui.Row(c).Gap(t.Space(2)).Wrap().Children(func() {
				if textButton(c, "创建项目", buttonAccent, plusIcon).Clicked() {
					a.openCreateDialog()
				}
				if textButton(c, "打开项目", buttonPlain, openIcon).Clicked() {
					a.openExistingProject()
				}
				if textButton(c, "从VCS获取", buttonPlain, gitIcon).Clicked() {
					a.openGitDialog()
				}
			})
			ui.SearchField(c, &a.search).Label("搜索项目").FillWidth()
		})
	})
}

func (a *app) welcomeList(c *ui.Context) {
	t := c.Theme()
	projects := a.cfg.filter(a.search)
	ui.Row(c).Fill().Justify(ui.Center).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(t.Rem(40)).Gap(t.Space(1.5)).MinHeight(0).Children(func() {
			ui.Row(c).Shrink(0).Gap(t.Space(1.5)).AlignItems(ui.Center).PaddingX(t.Space(1)).Children(func() {
				sectionLabel(c, "项目")
				ui.Text(c, fmt.Sprint(len(projects))).FontSize(t.Rem(0.78)).TextColor(t.TextMuted)
				ui.Box(c).Grow(1)
				if len(projects) > 0 {
					hintLabel(c, "单击打开，右键更多操作")
				}
			})
			if len(projects) == 0 {
				title, hint := "没有项目", "创建项目、打开本地目录或从 VCS 获取开始"
				if a.search != "" {
					title = "没有匹配的项目"
					hint = "试试其他关键词，或清除搜索条件"
				}
				card(c, ui.Box(c)).Grow(1).FillWidth().Children(func() {
					emptyState(c, searchIcon, title, hint)
				})
				return
			}
			ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).Gap(t.Space(1.5)).Children(func() {
					for _, p := range projects {
						a.projectRow(c, p)
					}
				})
			})
		})
	})
}

func (a *app) projectRow(c *ui.Context, p projectEntity) {
	t := c.Theme()
	missing := false
	if _, err := os.Stat(p.FilePath); err != nil {
		missing = true
	}
	row := ui.Row(c).Key(p.Name).FillWidth().Shrink(0).Gap(t.Space(2.5)).
		Padding(t.Space(2), t.Space(2.5)).AlignItems(ui.Center).
		Background(t.Surface).Radius(t.Radius).Cursor(ui.CursorPointer)
	if row.Hovered() && !missing {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		avatar(c, p.Name)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
			ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Name).FontSize(t.Rem(1.02)).Bold().SingleLine().Ellipsis("…")
				if missing {
					ui.Text(c, "路径不存在").FontSize(t.Rem(0.72)).TextColor(t.Danger).SingleLine()
				}
			})
			ui.Text(c, p.FilePath).FontSize(t.Rem(0.8)).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
			if !p.OpenedAt.IsZero() {
				ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, clockIcon).FontSize(iconSize(t, 0.7)).TextColor(t.TextMuted)
					ui.Text(c, "最近打开 "+humanTime(p.OpenedAt)).FontSize(t.Rem(0.72)).TextColor(t.TextMuted).SingleLine()
				})
			}
		})
		if row.Hovered() && !missing {
			ui.Icon(c, chevronIcon).FontSize(iconSize(t, 0.95)).TextColor(t.Accent)
		}
	})
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("打开").Chosen() {
			a.enterProject(p)
		}
		if m.Item("重命名").Chosen() {
			a.renameProject(p)
		}
		m.Separator()
		if m.Item("在文件管理器中显示").Chosen() {
			mygo.Shell.ShowItemInFolder(p.FilePath)
		}
		if m.Item("复制路径").Chosen() {
			a.copyToClipboard(p.FilePath)
		}
		m.Separator()
		if m.Item("从列表中移除").Chosen() {
			a.cfg.remove(p.FilePath)
			a.cfg.save()
		}
		if m.Item("删除当前项目").Chosen() {
			a.confirmDeleteProject(p)
		}
	})
	if row.Clicked() && !missing {
		a.enterProject(p)
	}
}

func avatar(c *ui.Context, name string) ui.Element {
	t := c.Theme()
	a := ui.Box(c).Size(t.Rem(2.4), t.Rem(2.4)).Background(hashColor(name)).
		Radius(t.Radius).Center().Shrink(0)
	a.Children(func() {
		ui.Text(c, strings.ToUpper(firstRune(name))).Bold().FontSize(t.Rem(1.05)).TextColor(ui.Hex("#ffffff"))
	})
	return a
}

func (a *app) renameProject(p projectEntity) {
	a.dlg.reset()
	a.dlg.inputOpen = true
	a.dlg.inputMode = inputRename
	a.dlg.inputTitle = "重命名项目"
	a.dlg.inputLabel = "新的项目名称"
	a.dlg.inputValue = p.Name
	a.dlg.renamePath = p.FilePath
	a.dlg.renameProject = true
	a.dlg.autoFocus = true
}

func humanTime(at time.Time) string {
	d := time.Since(at)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	default:
		return at.Format("2006-01-02")
	}
}

func (a *app) openExistingProject() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:     "请选择一个目录",
		Directory: true,
		Parent:    a.welcome,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	dir := filepath.Clean(paths[0])
	a.openProject(filepath.Base(dir), dir)
}

func (a *app) enterProject(p projectEntity) {
	a.openProject(p.Name, p.FilePath)
}

func (a *app) openProject(name, path string) {
	a.cfg.add(projectEntity{Name: name, FilePath: path})
	a.cfg.save()
	a.openMainWindow(name, path)
}

func hashColor(name string) ui.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return ui.Hex(avatarColors[h.Sum32()%uint32(len(avatarColors))])
}

func firstRune(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return "?"
	}
	return string(r)
}
