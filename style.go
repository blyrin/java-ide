package main

import (
	"github.com/egoist/mygo/ui"
)

func iconSize(t *ui.Theme, rem float32) float32 {
	return float32(int(t.Rem(rem) + 0.5))
}

func svgIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	javaIcon       = svgIcon(`<path d="M4 7c0-1.6 3.6-3 8-3s8 1.4 8 3-3.6 3-8 3-8-1.4-8-3z"/><path d="M4 7v5c0 1.6 3.6 3 8 3s8-1.4 8-3V7"/><path d="M4 12v5c0 1.6 3.6 3 8 3s8-1.4 8-3v-5"/>`)
	playIcon       = svgIcon(`<path d="M8.5 5.5v13l10-6.5z"/>`)
	buildIcon      = svgIcon(`<path d="M3.5 20h17M6 20v-5l6-4 6 4v5"/><path d="M10 20v-3h4v3"/>`)
	gearIcon       = svgIcon(`<circle cx="12" cy="12" r="2.8"/><path d="M12 2.8v2.4M12 18.8v2.4M2.8 12h2.4M18.8 12h2.4M5.5 5.5l1.7 1.7M16.8 16.8l1.7 1.7M18.5 5.5l-1.7 1.7M7.2 16.8l-1.7 1.7"/>`)
	infoIcon       = svgIcon(`<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5.5"/><circle cx="12" cy="7.8" r="0.9" fill="currentColor"/>`)
	plusIcon       = svgIcon(`<path d="M12 5v14M5 12h14"/>`)
	minusIcon      = svgIcon(`<path d="M5 12h14"/>`)
	openIcon       = svgIcon(`<path d="M4 19V6a2 2 0 0 1 2-2h4l2 2h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z"/>`)
	gitIcon        = svgIcon(`<circle cx="7" cy="6" r="2.4"/><circle cx="7" cy="18" r="2.4"/><circle cx="17" cy="12" r="2.4"/><path d="M7 8.4v7.2M9.4 6h3.6a3 3 0 0 1 3 3v.6"/>`)
	folderIcon     = svgIcon(`<path d="M4 19V6a2 2 0 0 1 2-2h4l2 2h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z"/>`)
	fileIcon       = svgIcon(`<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>`)
	trashIcon      = svgIcon(`<path d="M4 7h16M9.5 7V5.2A1.2 1.2 0 0 1 10.7 4h2.6a1.2 1.2 0 0 1 1.2 1.2V7M6.5 7l.8 12.1A1.9 1.9 0 0 0 9.2 21h5.6a1.9 1.9 0 0 0 1.9-1.9L17.5 7"/>`)
	searchIcon     = svgIcon(`<circle cx="11" cy="11" r="6.5"/><path d="M16 16l4.5 4.5"/>`)
	refreshIcon    = svgIcon(`<path d="M20 12a8 8 0 1 1-2.5-5.8"/><path d="M20 4v4h-4"/>`)
	branchIcon     = svgIcon(`<circle cx="7" cy="6" r="2.2"/><circle cx="7" cy="18" r="2.2"/><circle cx="17" cy="8" r="2.2"/><path d="M7 8.2v7.6M9.2 6h3.3a3 3 0 0 1 3 3v0"/>`)
	pullIcon       = svgIcon(`<path d="M12 4v11"/><path d="M7.5 11.5L12 16l4.5-4.5"/><path d="M5 20h14"/>`)
	pushIcon       = svgIcon(`<path d="M12 20V9"/><path d="M7.5 12.5L12 8l4.5 4.5"/><path d="M5 4h14"/>`)
	diffIcon       = svgIcon(`<path d="M4 6h6v12H4zM14 6h6v12h-6z"/><path d="M12 3v18"/>`)
	closeIcon      = svgIcon(`<path d="M6 6l12 12M18 6L6 18"/>`)
	structureIcon  = svgIcon(`<path d="M4 6h16M4 12h10M4 18h7"/><circle cx="18" cy="12" r="1.6"/><circle cx="15" cy="18" r="1.6"/>`)
	classIcon      = svgIcon(`<path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z"/><path d="M4 7.5l8 4.5 8-4.5M12 12v9"/>`)
	interfaceIcon  = svgIcon(`<circle cx="12" cy="12" r="8"/><path d="M8 12h8"/>`)
	enumIcon       = svgIcon(`<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M8 9h8M8 12h8M8 15h5"/>`)
	methodIcon     = svgIcon(`<path d="M6 4h6a3 3 0 0 1 3 3v13a2.5 2.5 0 0 0-2.5-2.5H6z"/><path d="M6 4v13.5"/>`)
	fieldIcon      = svgIcon(`<circle cx="12" cy="7" r="3"/><path d="M6 20v-2a6 6 0 0 1 12 0v2"/>`)
	problemIcon    = svgIcon(`<path d="M12 4l9 16H3z"/><path d="M12 10v4.5"/><circle cx="12" cy="17.2" r="0.9" fill="currentColor"/>`)
	consoleIcon    = svgIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l3 3-3 3M12.5 15h4"/>`)
	followIcon     = svgIcon(`<path d="M12 5v14"/><path d="M6 13l6 6 6-6"/>`)
	threadIcon     = svgIcon(`<circle cx="12" cy="5" r="2.4"/><path d="M12 7.4v6.1M12 13.5l-4 5.5M12 13.5l4 5.5M8 10.5h8"/>`)
	breakpointIcon = svgIcon(`<circle cx="12" cy="12" r="7"/><circle cx="12" cy="12" r="3.2" fill="currentColor"/>`)
	stepOverIcon   = svgIcon(`<path d="M5 6h14M5 12h9"/><path d="M14.5 15.5l3 2.5-3 2.5"/>`)
	stepIntoIcon   = svgIcon(`<path d="M5 6h14M5 12h6"/><path d="M14 15.5l3 2.5-3 2.5"/><path d="M11 12v3.5"/>`)
	stepOutIcon    = svgIcon(`<path d="M5 6h14M5 12h4"/><path d="M9 12v3.5"/><path d="M11 15.5l3 2.5-3 2.5"/>`)
	pauseIcon      = svgIcon(`<path d="M9 5v14M15 5v14"/>`)
	eyeIcon        = svgIcon(`<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="3"/>`)
	chevronIcon    = svgIcon(`<path d="M9 5l7 7-7 7"/>`)
	clockIcon      = svgIcon(`<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3.5 2"/>`)
	checkIcon      = svgIcon(`<path d="M4.5 12.5l5 5 10-11"/>`)
)

type buttonKind int

const (
	buttonPlain buttonKind = iota
	buttonAccent
	buttonDanger
)

func shadowOf(t *ui.Theme) ui.Color {
	if t.Dark {
		return t.Text.Alpha(0.5)
	}
	return t.Text.Alpha(0.13)
}

func hoverOf(t *ui.Theme) ui.Color { return t.Background.Mix(t.Text, 0.08) }

func activeOf(t *ui.Theme) ui.Color { return t.Background.Mix(t.Accent, 0.16) }

func textButton(c *ui.Context, label string, kind buttonKind, icon *ui.SVG) ui.Element {
	t := c.Theme()
	base, hover, fg := ui.Transparent, hoverOf(t), t.Text
	switch kind {
	case buttonAccent:
		base, hover, fg = t.Accent, t.AccentHover, t.AccentText
	case buttonDanger:
		base, hover, fg = t.Danger, t.Danger.Mix(t.Text, 0.18), t.AccentText
	}
	b := ui.ButtonBase(c).Padding(t.Space(1.25), t.Space(3)).TextColor(fg).Radius(t.Radius).
		Cursor(ui.CursorPointer).Label(label)
	bg := base
	if b.Hovered() || b.Pressed() {
		bg = hover
	}
	b.Background(bg)
	if kind == buttonAccent {
		b.Shadow(0, 1, t.Space(2), -t.Space(0.5), shadowOf(t))
	}
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(iconSize(t, 1.05))
		}
		ui.Text(c, label).SingleLine().Padding(0, 0, 0, t.Space(1))
	})
	return b
}

func iconButton(c *ui.Context, name string, icon *ui.SVG) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(t.Space(8), t.Space(8)).Center().TextColor(t.TextMuted).
		Radius(t.Radius).Cursor(ui.CursorPointer).Label(name).Tooltip(name)
	if b.Hovered() || b.Pressed() {
		b.Background(hoverOf(t)).TextColor(t.Text)
	} else {
		b.Background(ui.Transparent)
	}
	b.Children(func() {
		ui.Icon(c, icon).FontSize(iconSize(t, 1.1))
	})
	return b
}

func tabCloseButton(c *ui.Context, name string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(t.Rem(1.4), t.Rem(1.4)).Center().TextColor(t.TextMuted).
		Radius(t.Radius).Cursor(ui.CursorPointer).Tooltip("关闭").Label("关闭 " + name)
	if b.Hovered() || b.Pressed() {
		b.Background(t.Background.Mix(t.Text, 0.14)).TextColor(t.Text)
	} else {
		b.Background(ui.Transparent)
	}
	b.Children(func() {
		ui.Text(c, "×").FontSize(t.Rem(1))
	})
	return b
}

func badge(c *ui.Context, icon *ui.SVG, size float32) ui.Element {
	t := c.Theme()
	return badgeOn(c, icon, size, t.Accent, activeOf(t))
}

func badgeOn(c *ui.Context, icon *ui.SVG, size float32, fg, bg ui.Color) ui.Element {
	t := c.Theme()
	b := ui.Box(c).Size(size, size).Background(bg).Radius(t.Radius).Center()
	b.Children(func() {
		ui.Icon(c, icon).FontSize(float32(int(size*0.55 + 0.5))).TextColor(fg)
	})
	return b
}

func card(c *ui.Context, base ui.Element) ui.Element {
	t := c.Theme()
	return base.Background(t.Surface).Radius(t.Radius)
}

func panel(c *ui.Context, fn func()) ui.Element {
	p := ui.Box(c).Fill().Background(c.Theme().Surface)
	p.Children(fn)
	return p
}

func window(c *ui.Context) ui.Element {
	t := *c.Theme()
	t.Spacing, t.Radius, t.ScrollbarWidth = 4, 6, 8
	c.SetTheme(&t)
	return ui.Column(c).Fill().Background(t.Background)
}

func scrollContent(c *ui.Context, bottom float32, content func()) {
	t := c.Theme()
	ui.Scroll(c).Fill().Padding(t.Space(2.5), t.Space(2), bottom).
		ScrollbarInsets(t.Space(2.5), 0, bottom).Children(content)
}

func scrollPane(c *ui.Context, top, bottom float32, header, content func()) {
	t := c.Theme()
	ui.Scroll(c).Fill().Padding(top+t.Space(2.5), t.Space(2), bottom).
		ScrollbarInsets(top+t.Space(2.5), 0, bottom).Children(content)
	floatHeader(c, top, header)
}

func floatHeader(c *ui.Context, height float32, fn func()) {
	t := c.Theme()
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(height + t.Space(2)).
		PassThrough().Background(t.Surface)
	bar := ui.Row(c).Absolute().Top(t.Space(1)).Left(t.Space(2)).Right(t.Space(2)).Height(height).
		Padding(t.Space(0.5), t.Space(1)).Gap(t.Space(1.5)).AlignItems(ui.Center)
	bar.Children(fn)
}

func paneHeader(c *ui.Context, icon *ui.SVG, title string, trailing func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).FontSize(iconSize(t, 0.95)).TextColor(t.Accent)
		ui.Text(c, title).FontSize(t.Rem(0.85)).Bold().TextColor(t.TextMuted).SingleLine().
			Ellipsis("…").Grow(1).MinWidth(0)
		if trailing != nil {
			trailing()
		}
	})
}

func sectionLabel(c *ui.Context, text string) {
	t := c.Theme()
	ui.Text(c, text).FontSize(t.Rem(0.78)).Bold().TextColor(t.TextMuted).
		LetterSpacing(0.5).SingleLine()
}

func hintLabel(c *ui.Context, text string) {
	t := c.Theme()
	ui.Text(c, text).FontSize(t.Rem(0.8)).TextColor(t.TextMuted)
}

func emptyState(c *ui.Context, icon *ui.SVG, title, hint string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(t.Space(1.5)).Padding(t.Space(5)).Children(func() {
		badge(c, icon, t.Rem(3))
		ui.Text(c, title).FontSize(t.Rem(1.05)).Bold()
		if hint != "" {
			ui.Text(c, hint).FontSize(t.Rem(0.85)).TextColor(t.TextMuted).TextAlign(ui.Center)
		}
	})
}

func statusDot(c *ui.Context, color ui.Color) ui.Element {
	t := c.Theme()
	return ui.Box(c).Size(t.Space(2), t.Space(2)).Background(color).Radius(t.Space(1)).Shrink(0)
}

func pill(c *ui.Context, text string, color ui.Color) ui.Element {
	t := c.Theme()
	p := ui.Row(c).Shrink(0).Gap(t.Space(1.25)).AlignItems(ui.Center).
		Padding(t.Space(0.5), t.Space(1.75)).Background(t.Background.Mix(color, 0.15)).Radius(t.Space(2))
	p.Children(func() {
		statusDot(c, color)
		ui.Text(c, text).FontSize(t.Rem(0.75)).TextColor(color).SingleLine()
	})
	return p
}
