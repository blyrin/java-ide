package main

import (
	"log"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
)

type app struct {
	cfg     *appConfigure
	mono    string
	svc     ui.Services
	welcome *mygo.Window
	win     *mygo.Window
	search  string
	sess    *session
	dlg     dialogs
	menu    *mygo.Menu
	menuSig string
}

func (a *app) update(fn func()) {
	a.refreshMenuState()
	w := a.win
	if w == nil {
		w = a.welcome
	}
	if w == nil {
		fn()
		return
	}
	w.Update(fn)
}

func (a *app) openWelcomeWindow() {
	if s := a.sess; s != nil {
		s.close()
	}
	a.sess = nil
	a.search = ""
	a.dlg.reset()
	a.win = nil
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "欢迎使用 Java IDE",
		Width:     780,
		Height:    820,
		MinWidth:  620,
		MinHeight: 640,
		StateKey:  "welcome",
		Content:   ui.View(a.welcomeView),
	})
	a.welcome = w
	w.OnClose(func(e *mygo.CloseEvent) { a.cfg.save() })
}

func (a *app) openMainWindow(name, path string) {
	a.dlg.reset()
	s := newSession(a, name, path, a.mono)
	a.sess = s
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "项目：" + name,
		Width:     1180,
		Height:    820,
		MinWidth:  720,
		MinHeight: 620,
		StateKey:  "main",
		Content:   ui.View(a.mainView),
	})
	a.win = w
	a.menu = a.mainMenu()
	w.SetMenu(a.menu)
	w.OnClose(func(e *mygo.CloseEvent) {
		s.close()
		a.cfg.save()
		a.openWelcomeWindow()
	})
	if a.welcome != nil {
		a.welcome.Close()
		a.welcome = nil
	}
	s.startWatch()
	s.gitRefresh()
	s.indexSymbols()
}

func main() {
	a := &app{cfg: loadConfigure()}
	mygo.Theme.SetSource(a.cfg.settings().themeSource())
	mygo.App.WhenReady(func() {
		mygo.App.SetLocale("zh-Hans")
		a.mono = a.cfg.settings().monoFamily()
		a.openWelcomeWindow()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func monoFamily() string {
	for _, f := range ui.FontFamilies() {
		if f.Monospaced {
			return f.Name
		}
	}
	return ""
}
