package main

import (
	"path/filepath"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const (
	debugTabFrames = iota
	debugTabVars
	debugTabWatches
	debugTabThreads
	debugTabBreakpoints
)

var debugTabNames = []string{"帧", "变量", "监视", "线程", "断点"}

func (a *app) debugPane(c *ui.Context, s *session) {
	t := c.Theme()
	box := ui.Box(c).Fill().Background(t.Surface)
	box.Children(func() {
		ui.Box(c).FillWidth().Shrink(0).Children(func() {
			ui.Segmented(c, &s.debugTab, debugTabNames...)
		})
		body := ui.Box(c).Fill()
		body.Children(func() {
			switch s.debugTab {
			case debugTabVars:
				a.debugVarsPane(c, s)
			case debugTabWatches:
				a.debugWatchesPane(c, s)
			case debugTabThreads:
				a.debugThreadsPane(c, s)
			case debugTabBreakpoints:
				a.debugBreakpointsPane(c, s)
			default:
				a.debugFramesPane(c, s)
			}
		})
	})
}

func (a *app) debugIdle(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(t.Space(1.5)).Padding(t.Space(4)).Children(func() {
		badge(c, threadIcon, t.Rem(2.6))
		ui.Text(c, "尚未开始调试").Bold()
		ui.Text(c, "通过「调试 → 开始调试」启动调试会话").FontSize(t.Rem(0.8)).TextColor(t.TextMuted).
			TextAlign(ui.Center)
	})
}

func (a *app) debugNotSuspended(c *ui.Context, s *session) {
	t := c.Theme()
	text, hint := "程序未暂停", ""
	if d := s.dbg; d != nil {
		switch d.currentState() {
		case debugLaunching:
			text, hint = "正在启动调试会话…", "等待与调试目标建立连接"
		case debugRunning:
			text, hint = "程序运行中", "命中断点、暂停或单步后显示数据"
		}
	}
	ui.Column(c).Grow(1).Center().Gap(t.Space(1)).Padding(t.Space(4)).Children(func() {
		if d := s.dbg; d != nil && d.currentState() == debugLaunching {
			ui.Spinner(c)
		} else {
			badge(c, pauseIcon, t.Rem(2.6))
		}
		ui.Text(c, text).Bold()
		if hint != "" {
			ui.Text(c, hint).FontSize(t.Rem(0.8)).TextColor(t.TextMuted).TextAlign(ui.Center)
		}
	})
}

func (a *app) debugFramesPane(c *ui.Context, s *session) {
	t := c.Theme()
	d := s.dbg
	if d == nil {
		a.debugIdle(c)
		return
	}
	state, frames, frame, _, _, _, _, _ := d.snapshot()
	if state != debugSuspended || len(frames) == 0 {
		a.debugNotSuspended(c, s)
		return
	}
	ui.List(c, &s.debugFramesList, len(frames)).Grow(1).FillWidth().Gap(t.Space(0.5)).PaddingY(t.Space(1)).
		Rows(func(r ui.ListRow) {
			f := frames[r.Index]
			row := ui.Column(r.Context).FillWidth().Gap(0).Padding(t.Space(0.5), t.Space(1)).
				Radius(t.Radius)
			if r.Index == frame {
				row.Background(activeOf(t))
			}
			row.Children(func() {
				ui.Row(r.Context).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					ui.Icon(r.Context, methodIcon).FontSize(iconSize(t, 0.8)).TextColor(t.Accent)
					ui.Text(r.Context, f.class+"."+f.name).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
				})
				ui.Text(r.Context, relativeTo(s.path, f.file)+":"+strconv.Itoa(f.line)).
					FontSize(t.Rem(0.75)).TextColor(t.TextMuted).SingleLine()
			})
			if row.Clicked() {
				d.selectFrame(r.Index)
			}
		})
}

func (a *app) debugVarsPane(c *ui.Context, s *session) {
	t := c.Theme()
	d := s.dbg
	if d == nil {
		a.debugIdle(c)
		return
	}
	state, _, _, vars, byKey, _, _, _ := d.snapshot()
	if state != debugSuspended {
		a.debugNotSuspended(c, s)
		return
	}
	if len(vars) == 0 {
		ui.Text(c, "没有可显示的变量").TextColor(t.TextMuted).PaddingX(t.Space(1.5))
		return
	}
	roots := make([]string, 0, len(vars))
	for _, v := range vars {
		roots = append(roots, v.key)
	}
	ui.Outline(c, &s.debugVars, roots, func(key string) []string {
		v := byKey[key]
		if v == nil || !v.expandable || !v.loaded {
			return nil
		}
		if len(v.children) == 0 {
			return []string{}
		}
		out := make([]string, 0, len(v.children))
		for _, ch := range v.children {
			out = append(out, ch.key)
		}
		return out
	}, func(key string) {
		v := byKey[key]
		if v == nil {
			return
		}
		if v.expandable && !v.loaded && s.debugVars.Open.Has(key) {
			d.expandVar(v)
		}
		row := ui.Column(c).FillWidth().Gap(0).Padding(t.Space(0.5), t.Space(1)).Radius(t.Radius)
		row.Children(func() {
			ui.Row(c).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, v.name).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
				if v.typeName != "" {
					ui.Text(c, v.typeName).FontSize(t.Rem(0.75)).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
			val := ui.Text(c, v.value).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…")
			if a.mono != "" {
				val.Font(a.mono)
			}
			val.TextColor(t.Text)
		})
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item("复制值").Chosen() {
				a.copyToClipboard(v.value)
			}
			if m.Item("复制名称").Chosen() {
				a.copyToClipboard(v.name)
			}
		})
	}).FillWidth().Grow(1)
}

func (a *app) debugWatchesPane(c *ui.Context, s *session) {
	t := c.Theme()
	d := s.dbg
	if d == nil {
		a.debugIdle(c)
		return
	}
	state, _, _, _, _, _, watches, vals := d.snapshot()
	if len(watches) == 0 {
		ui.Column(c).Gap(t.Space(1)).Padding(t.Space(1.5)).Children(func() {
			ui.Text(c, "没有监视表达式").TextColor(t.TextMuted)
			ui.Text(c, "通过「调试 → 添加监视表达式」或「计算表达式」添加").FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
		return
	}
	if len(vals) != len(watches) {
		vals = make([]string, len(watches))
		for i := range vals {
			if state == debugSuspended {
				vals[i] = "计算中…"
			} else {
				vals[i] = "程序未暂停"
			}
		}
	}
	ui.List(c, &s.debugWatchesList, len(watches)).Grow(1).FillWidth().Gap(t.Space(0.5)).PaddingY(t.Space(1)).
		Rows(func(r ui.ListRow) {
			row := ui.Column(r.Context).FillWidth().Gap(0).Padding(t.Space(0.5), t.Space(1))
			row.Children(func() {
				expr := ui.Text(r.Context, watches[r.Index]).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…")
				if a.mono != "" {
					expr.Font(a.mono)
				}
				val := ui.Text(r.Context, vals[r.Index]).FontSize(t.Rem(0.8)).SingleLine().Ellipsis("…")
				if a.mono != "" {
					val.Font(a.mono)
				}
				val.TextColor(t.TextMuted)
			})
			row.ContextMenu(func(m *ui.Menu) {
				i := r.Index
				if m.Item("移除").Chosen() {
					d.removeWatch(i)
				}
				if m.Item("复制值").Chosen() {
					a.copyToClipboard(vals[i])
				}
			})
		})
}

func (a *app) debugThreadsPane(c *ui.Context, s *session) {
	t := c.Theme()
	d := s.dbg
	if d == nil {
		a.debugIdle(c)
		return
	}
	state, _, _, _, _, threads, _, _ := d.snapshot()
	if state != debugSuspended {
		a.debugNotSuspended(c, s)
		return
	}
	d.mu.Lock()
	current := d.thread
	d.mu.Unlock()
	if len(threads) == 0 {
		ui.Text(c, "没有线程").TextColor(t.TextMuted).PaddingX(t.Space(1.5))
		return
	}
	ui.List(c, &s.debugThreadsList, len(threads)).Grow(1).FillWidth().Gap(t.Space(0.5)).PaddingY(t.Space(1)).
		Rows(func(r ui.ListRow) {
			th := threads[r.Index]
			row := ui.Row(r.Context).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).
				Padding(t.Space(0.5), t.Space(1)).Radius(t.Radius)
			if th.id == current {
				row.Background(activeOf(t))
			}
			row.Children(func() {
				ui.Icon(r.Context, threadIcon).FontSize(iconSize(t, 0.8)).TextColor(t.TextMuted)
				ui.Text(r.Context, th.name).FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
				ui.Text(r.Context, threadStatusText(th.status)).FontSize(t.Rem(0.75)).TextColor(t.TextMuted).SingleLine()
			})
			if row.Clicked() {
				d.selectThread(th.id)
			}
		})
}

func (a *app) debugBreakpointsPane(c *ui.Context, s *session) {
	t := c.Theme()
	bps := s.breakpoints
	if len(bps) == 0 {
		ui.Column(c).Gap(t.Space(1)).Padding(t.Space(1.5)).Children(func() {
			ui.Text(c, "没有断点").TextColor(t.TextMuted)
			ui.Text(c, "在编辑器中点击行号槽即可设置断点").FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
		})
		return
	}
	ui.List(c, &s.debugBreaksList, len(bps)).Grow(1).FillWidth().Gap(t.Space(0.5)).PaddingY(t.Space(1)).
		Rows(func(r ui.ListRow) {
			bp := bps[r.Index]
			row := ui.Column(r.Context).FillWidth().Gap(0).Padding(t.Space(0.5), t.Space(1))
			row.Children(func() {
				ui.Row(r.Context).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					ui.Icon(r.Context, breakpointIcon).FontSize(iconSize(t, 0.8)).
						TextColor(map[bool]ui.Color{true: t.Danger, false: t.TextMuted}[bp.Enabled])
					ui.Text(r.Context, filepath.Base(bp.File)+":"+strconv.Itoa(bp.Line)).
						FontSize(t.Rem(0.85)).SingleLine().Ellipsis("…").Grow(1)
				})
				ui.Text(r.Context, relativeTo(s.path, bp.File)).FontSize(t.Rem(0.75)).TextColor(t.TextMuted).SingleLine()
				if bp.Condition != "" {
					cond := ui.Text(r.Context, "条件："+bp.Condition).FontSize(t.Rem(0.75)).SingleLine().Ellipsis("…")
					if a.mono != "" {
						cond.Font(a.mono)
					}
					cond.TextColor(t.Accent)
				}
			})
			row.ContextMenu(func(m *ui.Menu) {
				bp := bps[r.Index]
				if m.Item("启用").Chosen() {
					s.setBreakpointEnabled(bp.File, bp.Line, true)
				}
				if m.Item("禁用").Chosen() {
					s.setBreakpointEnabled(bp.File, bp.Line, false)
				}
				m.Separator()
				if m.Item("编辑条件").Chosen() {
					a.openConditionDialog(bp)
				}
				if m.Item("跳转到断点").Chosen() {
					a.openFileAt(bp.File, bp.Line, 0)
				}
				m.Separator()
				if m.Item("删除").Chosen() {
					s.removeBreakpoint(bp)
				}
			})
			if row.Clicked() {
				a.openFileAt(bp.File, bp.Line, 0)
			}
		})
}

func threadStatusText(st int) string {
	switch st {
	case statusZombie:
		return "已结束"
	case statusRunning:
		return "运行中"
	case statusSleeping:
		return "休眠"
	case statusMonitor:
		return "等待监视器"
	case statusWait:
		return "等待"
	}
	return "未知"
}

func (s *session) applyDebugMarks(t *ui.Theme, tb *editorTab) {
	if tb == nil || tb.ed == nil {
		return
	}
	tb.ed.GutterMarks = nil
	tb.ed.LineBackgrounds = nil
	for _, bp := range s.breakpoints {
		if bp.File == "" || filepath.Clean(bp.File) != filepath.Clean(tb.path) {
			continue
		}
		color := t.Danger
		if !bp.Enabled {
			color = t.TextMuted
		}
		tb.ed.GutterMarks = append(tb.ed.GutterMarks, editor.GutterMark{Line: max(bp.Line-1, 0), Color: color})
	}
	if d := s.dbg; d != nil {
		if file, line := d.current(); file != "" && line > 0 && filepath.Clean(file) == filepath.Clean(tb.path) {
			tb.ed.LineBackgrounds = append(tb.ed.LineBackgrounds, editor.LineBackground{
				First: max(line-1, 0), Last: max(line-1, 0), Color: t.Background.Mix(t.Warning, 0.3),
			})
		}
	}
}
