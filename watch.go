package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

func (s *session) startWatch() {
	ctx, cancel := context.WithCancel(context.Background())
	w, err := mygo.WatchFiles(ctx, s.path, mygo.FileWatchOptions{
		Recursive: true,
		Debounce:  120 * time.Millisecond,
		MaxDelay:  time.Second,
		Ignore: func(path string, isDir bool) bool {
			name := path
			if _, after, ok := strings.CutLast(path, "/"); ok {
				name = after
			}
			if isDir {
				return skipDirs[name]
			}
			return name == ".ide.json"
		},
	})
	if err != nil {
		cancel()
		return
	}
	s.watchStop = cancel
	app := s.app
	go func() {
		for {
			if _, err := w.Next(ctx); err != nil {
				return
			}
			app.update(func() {
				s.refreshTree()
				if s.sideTab == sideGit {
					s.gitRefresh()
				}
			})
		}
	}()
}
