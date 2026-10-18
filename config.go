package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const appDirName = "java-ide"

type projectEntity struct {
	Name     string    `json:"name"`
	FilePath string    `json:"filePath"`
	OpenedAt time.Time `json:"openedAt"`
}

type appConfigure struct {
	Projects []projectEntity `json:"projects"`
	Settings appSettings     `json:"settings"`
}

type runConfig struct {
	Name        string   `json:"name"`
	MainClass   string   `json:"mainClass"`
	VMArgs      []string `json:"vmArgs,omitempty"`
	ProgramArgs []string `json:"programArgs,omitempty"`
	Env         []string `json:"env,omitempty"`
	WorkDir     string   `json:"workDir,omitempty"`
	BuildTool   string   `json:"buildTool,omitempty"`
}

type projectConfigure struct {
	MainClass   string            `json:"mainClass"`
	JavaCommand string            `json:"javaCommand"`
	RunConfigs  []runConfig       `json:"runConfigs,omitempty"`
	ActiveRun   int               `json:"activeRun"`
	BuildTool   string            `json:"buildTool,omitempty"`
	Encoding    string            `json:"encoding,omitempty"`
	Breakpoints []debugBreakpoint `json:"breakpoints,omitempty"`
}

func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, appDirName)
}

func configFile() string { return filepath.Join(configDir(), "config.json") }

func loadConfigure() *appConfigure {
	cfg := &appConfigure{}
	b, err := os.ReadFile(configFile())
	if err != nil {
		cfg.Settings = defaultSettings()
		return cfg
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return &appConfigure{Settings: defaultSettings()}
	}
	if cfg.Projects == nil {
		cfg.Projects = []projectEntity{}
	}
	cfg.Settings = cfg.Settings.normalized()
	if !settingsKeyPresent(b, "columnGuide") {
		cfg.Settings.ColumnGuide = true
	}
	return cfg
}

func settingsKeyPresent(b []byte, key string) bool {
	var raw struct {
		Settings map[string]json.RawMessage `json:"settings"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return true
	}
	_, ok := raw.Settings[key]
	return ok
}

func (cfg *appConfigure) save() {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(configFile(), b, 0o644)
}

func (cfg *appConfigure) settings() appSettings {
	if cfg.Settings == (appSettings{}) {
		return defaultSettings()
	}
	return cfg.Settings.normalized()
}

func (cfg *appConfigure) add(p projectEntity) {
	if p.OpenedAt.IsZero() {
		p.OpenedAt = time.Now()
	}
	for i, e := range cfg.Projects {
		if e.FilePath == p.FilePath {
			cfg.Projects[i] = p
			return
		}
	}
	cfg.Projects = append(cfg.Projects, p)
}

func (cfg *appConfigure) remove(path string) {
	kept := make([]projectEntity, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		if p.FilePath != path {
			kept = append(kept, p)
		}
	}
	cfg.Projects = kept
}

func (cfg *appConfigure) rename(path, name string) {
	for i, p := range cfg.Projects {
		if p.FilePath == path {
			cfg.Projects[i].Name = name
			return
		}
	}
}

func (cfg *appConfigure) filter(search string) []projectEntity {
	search = strings.ToLower(strings.TrimSpace(search))
	out := make([]projectEntity, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		if search == "" || strings.Contains(strings.ToLower(p.Name), search) ||
			strings.Contains(strings.ToLower(filepath.Base(p.FilePath)), search) ||
			strings.Contains(strings.ToLower(filepath.ToSlash(p.FilePath)), search) {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(a, b projectEntity) int {
		switch {
		case a.OpenedAt.After(b.OpenedAt):
			return -1
		case a.OpenedAt.Before(b.OpenedAt):
			return 1
		default:
			return strings.Compare(a.Name, b.Name)
		}
	})
	return out
}

func (cfg *projectConfigure) activeRun() runConfig {
	if cfg.ActiveRun >= 0 && cfg.ActiveRun < len(cfg.RunConfigs) {
		return cfg.RunConfigs[cfg.ActiveRun]
	}
	return runConfig{Name: "当前配置"}
}

func (cfg *projectConfigure) setRun(rc runConfig) {
	if cfg.ActiveRun >= 0 && cfg.ActiveRun < len(cfg.RunConfigs) {
		cfg.RunConfigs[cfg.ActiveRun] = rc
		return
	}
	cfg.RunConfigs = append(cfg.RunConfigs, rc)
	cfg.ActiveRun = len(cfg.RunConfigs) - 1
}

func (rc runConfig) mainClassOrDefault(fallback string) string {
	if strings.TrimSpace(rc.MainClass) != "" {
		return strings.TrimSpace(rc.MainClass)
	}
	return fallback
}

func (rc runConfig) environment() []string {
	out := make([]string, 0, len(rc.Env))
	for _, e := range rc.Env {
		if strings.TrimSpace(e) != "" {
			out = append(out, strings.TrimSpace(e))
		}
	}
	return out
}

func (rc runConfig) programArguments() []string {
	var out []string
	for _, a := range rc.ProgramArgs {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func (rc runConfig) vmArguments() []string {
	var out []string
	for _, a := range rc.VMArgs {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}
