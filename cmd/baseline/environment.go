package main

import (
	"os"
	"path/filepath"
	"strings"
)

func isolateEnvironment(root string) func() {
	values := map[string]string{
		"HOME":                filepath.Join(root, "home"),
		"XDG_CONFIG_HOME":     filepath.Join(root, "config"),
		"XDG_STATE_HOME":      filepath.Join(root, "state"),
		"XDG_CACHE_HOME":      filepath.Join(root, "cache"),
		"XDG_DATA_HOME":       filepath.Join(root, "data"),
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_SYSTEM":   os.DevNull,
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_AUTHOR_NAME":     "Baseline",
		"GIT_AUTHOR_EMAIL":    "baseline@example.invalid",
		"GIT_COMMITTER_NAME":  "Baseline",
		"GIT_COMMITTER_EMAIL": "baseline@example.invalid",
	}
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		must(os.MkdirAll(values[key], 0700))
	}
	type setting struct {
		value   string
		present bool
	}
	saved := make(map[string]setting)
	for key := range values {
		value, present := os.LookupEnv(key)
		saved[key] = setting{value, present}
	}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			saved[key] = setting{value, true}
			must(os.Unsetenv(key))
		}
	}
	for key, value := range values {
		must(os.Setenv(key, value))
	}
	return func() {
		for key, original := range saved {
			if original.present {
				must(os.Setenv(key, original.value))
			} else {
				must(os.Unsetenv(key))
			}
		}
	}
}
