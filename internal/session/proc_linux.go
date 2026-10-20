//go:build linux

package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Read once per probe, not once per session. Commands may include spaces or ')'.
func processes() processTable {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	table := make(processTable)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndexByte(string(data), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(data[end+1:]))
		if len(fields) < 20 || len(fields[0]) != 1 {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		sid, sidErr := strconv.Atoi(fields[3])
		start, startErr := strconv.ParseUint(fields[19], 10, 64)
		if err == nil && sidErr == nil && startErr == nil {
			table[pid] = processState{ppid: ppid, state: fields[0][0], sid: sid, start: start, timer: timerProcess(data, end)}
		}
	}
	return table
}

func timerProcess(stat []byte, end int) bool {
	return commandName(stat, end) == "sleep"
}

func enrichTimer(pid int, state processState) processState {
	if state.timer || state.state != 'S' {
		return state
	}
	name, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "wchan"))
	if err == nil && timerWait(string(name)) {
		state.timer = true
	}
	return state
}

func commandName(stat []byte, end int) string {
	open := bytes.IndexByte(stat, '(')
	if open < 0 || end <= open {
		return ""
	}
	return string(stat[open+1 : end])
}

// sessionMarked reports whether pid's environment contains the exact entry.
func sessionMarked(pid int, marker []byte) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return false
	}
	for len(data) > 0 {
		value, rest, _ := bytes.Cut(data, []byte{0})
		if bytes.Equal(value, marker) {
			return true
		}
		data = rest
	}
	return false
}
