package main

import (
	_ "embed"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/1.mp4
var eggVideo []byte

const (
	eggClickLimit = 5
	eggVideoFile  = "1.mp4"
	eggPageFile   = "egg.html"
)

const eggPage = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
html, body { margin: 0; height: 100%; background: #000; overflow: hidden; }
video { width: 100%; height: 100%; object-fit: contain; background: #000; }
</style>
</head>
<body>
<video id="egg" src="1.mp4" autoplay></video>
<script>
var v = document.getElementById('egg');
v.volume = 0.5;
v.play().catch(function () { v.muted = true; v.play().catch(function () {}); });
</script>
</body>
</html>
`

func (a *app) eggLogoClick() {
	a.eggClicks++
	if a.eggClicks < eggClickLimit {
		return
	}
	a.eggClicks = 0
	a.update(a.playEgg)
}

func (a *app) playEgg() {
	if a.eggWin != nil {
		a.eggWin.Focus()
		return
	}
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS",
		"--autoplay-policy=no-user-gesture-required --allow-file-access-from-files")
	dir, err := os.MkdirTemp("", "java-ide-egg-")
	if err != nil {
		return
	}
	drop := func() {
		a.eggWin = nil
		a.eggWeb = nil
		os.RemoveAll(dir)
	}
	if err := os.WriteFile(filepath.Join(dir, eggVideoFile), eggVideo, 0o600); err != nil {
		os.RemoveAll(dir)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, eggPageFile), []byte(eggPage), 0o600); err != nil {
		os.RemoveAll(dir)
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "会赢的",
		Width:     1280,
		Height:    720,
		MinWidth:  480,
		MinHeight: 320,
		StateKey:  "egg",
		Content:   ui.View(a.eggView),
	})
	v, err := w.NewWebView(mygo.WebViewOptions{BackgroundColor: "#000000"})
	if err != nil {
		w.Close()
		mygo.Shell.OpenExternal(fileURL(filepath.Join(dir, eggVideoFile)))
		os.RemoveAll(dir)
		return
	}
	a.eggWin = w
	a.eggWeb = v
	w.OnClose(func(e *mygo.CloseEvent) { drop() })
	v.Page().LoadFile(filepath.Join(dir, eggPageFile))
}

func (a *app) eggView(c *ui.Context) {
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		if a.eggWeb != nil {
			ui.WebView(c, a.eggWeb).Grow(1)
		}
	})
}

func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
