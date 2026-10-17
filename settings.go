package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	themeSystem = "system"
	themeLight  = "light"
	themeDark   = "dark"
)

const (
	buildAuto   = "auto"
	buildJavac  = "javac"
	buildMaven  = "maven"
	buildGradle = "gradle"
)

type appSettings struct {
	Theme         string  `json:"theme"`
	FontSize      float32 `json:"fontSize"`
	Mono          string  `json:"mono"`
	AutoSave      bool    `json:"autoSave"`
	ShowHidden    bool    `json:"showHidden"`
	TabSize       int     `json:"tabSize"`
	SoftWrap      bool    `json:"softWrap"`
	LineNumbers   bool    `json:"lineNumbers"`
	ColumnGuide   bool    `json:"columnGuide"`
	BuildTool     string  `json:"buildTool"`
	JdkHome       string  `json:"jdkHome"`
	MavenCommand  string  `json:"mavenCommand"`
	GradleCommand string  `json:"gradleCommand"`
	Encoding      string  `json:"encoding"`
	GitCommand    string  `json:"gitCommand"`
	ConsoleFollow bool    `json:"consoleFollow"`
	ConfirmDelete bool    `json:"confirmDelete"`
}

func defaultSettings() appSettings {
	return appSettings{
		Theme: themeSystem, FontSize: 13, AutoSave: true, TabSize: 4,
		LineNumbers: true, ColumnGuide: true, BuildTool: buildAuto, Encoding: "UTF-8",
		ConsoleFollow: true, ConfirmDelete: true,
	}
}

func (s appSettings) normalized() appSettings {
	if s.Theme == "" {
		s.Theme = themeSystem
	}
	if s.FontSize <= 0 {
		s.FontSize = 13
	}
	if s.TabSize <= 0 {
		s.TabSize = 4
	}
	if s.BuildTool == "" {
		s.BuildTool = buildAuto
	}
	if s.Encoding == "" {
		s.Encoding = "UTF-8"
	}
	return s
}

func (s appSettings) themeSource() mygo.ThemeSource {
	switch s.Theme {
	case themeLight:
		return mygo.ThemeLight
	case themeDark:
		return mygo.ThemeDark
	default:
		return mygo.ThemeSystem
	}
}

func (s appSettings) monoFamily() string {
	if s.Mono != "" {
		return s.Mono
	}
	return monoFamily()
}

var (
	themeLabels = []string{"跟随系统", "浅色", "深色"}
	themeValues = []string{themeSystem, themeLight, themeDark}
	buildLabels = []string{"自动检测", "javac", "Maven", "Gradle"}
	buildValues = []string{buildAuto, buildJavac, buildMaven, buildGradle}
)

func optionLabel(labels, values []string, value string) string {
	for i, v := range values {
		if v == value {
			return labels[i]
		}
	}
	return labels[0]
}

func optionValue(labels, values []string, label string) string {
	for i, l := range labels {
		if l == label {
			return values[i]
		}
	}
	return values[0]
}

func monoOptions() []string {
	out := []string{"系统默认"}
	for _, f := range ui.FontFamilies() {
		if f.Monospaced {
			out = append(out, f.Name)
		}
	}
	return out
}

func (a *app) openSettingsDialog() {
	d := &a.dlg
	d.reset()
	d.settingsOpen = true
	d.settingsValues = a.cfg.settings()
	d.settingsTheme = optionLabel(themeLabels, themeValues, d.settingsValues.Theme)
	d.settingsBuild = optionLabel(buildLabels, buildValues, d.settingsValues.BuildTool)
	d.settingsMono = d.settingsValues.Mono
	if d.settingsMono == "" {
		d.settingsMono = "系统默认"
	}
	d.autoFocus = true
}

func (a *app) applySettings() {
	a.cfg.Settings = a.dlg.settingsValues.normalized()
	a.refreshMenuState()
	if mygo.App.IsReady() {
		mygo.Theme.SetSource(a.cfg.Settings.themeSource())
	}
	a.mono = a.cfg.Settings.monoFamily()
	if s := a.sess; s != nil {
		s.applySettings(a.cfg.Settings)
	}
	a.cfg.save()
}

func (a *app) settingsDialog(c *ui.Context) {
	d := &a.dlg
	if !d.settingsOpen {
		return
	}
	a.dialog(c, &d.settingsOpen, 520, 0, func() {
		t := c.Theme()
		dialogTitle(c, gearIcon, "设置", 1.3)
		ui.Scroll(c).Grow(1).FillWidth().MaxHeight(t.Rem(28)).Children(func() {
			ui.Column(c).Gap(t.Space(3.5)).Children(func() {
				a.settingsSection(c, "外观", func() {
					ui.Field(c, "主题", func() {
						ui.Select(c, &d.settingsTheme, themeLabels).Width(t.Space(28)).Label("主题")
						d.settingsValues.Theme = optionValue(themeLabels, themeValues, d.settingsTheme)
					})
					ui.Field(c, "等宽字体", func() {
						ui.Select(c, &d.settingsMono, monoOptions()).Width(t.Space(28)).Label("等宽字体")
						if d.settingsMono == "系统默认" {
							d.settingsValues.Mono = ""
						} else {
							d.settingsValues.Mono = d.settingsMono
						}
					})
					ui.Field(c, "编辑器字号", func() {
						size := float64(d.settingsValues.FontSize)
						ui.NumberInput(c, &size, 9, 24, 1).Width(t.Space(28)).Label("编辑器字号")
						d.settingsValues.FontSize = float32(size)
					})
				})
				a.settingsSection(c, "编辑器", func() {
					ui.Checkbox(c, &d.settingsValues.LineNumbers, "显示行号")
					ui.Checkbox(c, &d.settingsValues.ColumnGuide, "显示 80 列参考线")
					ui.Checkbox(c, &d.settingsValues.SoftWrap, "软换行")
					ui.Checkbox(c, &d.settingsValues.AutoSave, "修改后自动保存")
					ui.Field(c, "Tab 宽度", func() {
						size := float64(d.settingsValues.TabSize)
						ui.NumberInput(c, &size, 2, 8, 1).Width(t.Space(28)).Label("Tab 宽度")
						d.settingsValues.TabSize = int(size)
					})
				})
				a.settingsSection(c, "构建与工具", func() {
					ui.Field(c, "构建工具", func() {
						ui.Select(c, &d.settingsBuild, buildLabels).Width(t.Space(28)).Label("构建工具")
						d.settingsValues.BuildTool = optionValue(buildLabels, buildValues, d.settingsBuild)
					})
					ui.Field(c, "JDK 主目录", func() {
						ui.TextInput(c, &d.settingsValues.JdkHome).Placeholder("留空使用 PATH 中的 JDK").
							Grow(1).Label("JDK 主目录")
					})
					ui.Field(c, "源码编码", func() {
						ui.TextInput(c, &d.settingsValues.Encoding).Placeholder("UTF-8").
							Width(t.Space(28)).Label("源码编码")
					})
					ui.Field(c, "Maven 命令", func() {
						ui.TextInput(c, &d.settingsValues.MavenCommand).Placeholder("mvn").
							Grow(1).Label("Maven 命令")
					})
					ui.Field(c, "Gradle 命令", func() {
						ui.TextInput(c, &d.settingsValues.GradleCommand).Placeholder("gradle").
							Grow(1).Label("Gradle 命令")
					})
					ui.Field(c, "Git 命令", func() {
						ui.TextInput(c, &d.settingsValues.GitCommand).Placeholder("git").
							Grow(1).Label("Git 命令")
					})
				})
				a.settingsSection(c, "项目与控制台", func() {
					ui.Checkbox(c, &d.settingsValues.ShowHidden, "文件树显示隐藏文件")
					ui.Checkbox(c, &d.settingsValues.ConsoleFollow, "控制台自动跟随输出")
					ui.Checkbox(c, &d.settingsValues.ConfirmDelete, "删除项目时需要确认")
				})
			})
		})
		dialogButtons(c, func() {
			if textButton(c, "取消", buttonPlain, nil).Clicked() {
				d.settingsOpen = false
			}
			if textButton(c, "应用", buttonAccent, checkIcon).Clicked() {
				a.applySettings()
				d.settingsOpen = false
			}
		})
	})
}

func (a *app) settingsSection(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(1.5)).Children(func() {
		sectionLabel(c, title)
		ui.Form(c, fn)
	})
}
