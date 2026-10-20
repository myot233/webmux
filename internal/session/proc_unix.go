//go:build linux || darwin

package session

// state uses ps letters: R running, S sleeping, Z zombie. Other values, including
// '?' for a process whose run state is unreadable, never count as waiting.
type processState struct {
	ppid  int
	state byte
	sid   int
	start uint64
}
type processTable map[int]processState

// A probe's immutable index is shared by every session until the next probe.
type processSnapshot struct {
	table    processTable
	children map[int][]int
}

func newProcessSnapshot(table processTable) processSnapshot {
	children := make(map[int][]int, len(table))
	for child, state := range table {
		children[state.ppid] = append(children[state.ppid], child)
	}
	return processSnapshot{table: table, children: children}
}

func (p processSnapshot) foregroundBlocked(pid int) bool {
	root, ok := p.table[pid]
	if !ok {
		return false
	}
	if root.state == 'R' {
		return false
	}
	seen := map[int]bool{pid: true}
	pending := []int{pid}
	leaves := 0
	for len(pending) > 0 {
		member := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		state := p.table[member]
		if state.state == 'R' {
			return false
		}
		if len(p.children[member]) == 0 {
			leaves++
			if state.state != 'S' {
				return false
			}
		}
		for _, child := range p.children[member] {
			// Process snapshots can contain cycles; visit each PID only once.
			if !seen[child] {
				seen[child] = true
				pending = append(pending, child)
			}
		}
	}
	return leaves > 0
}
