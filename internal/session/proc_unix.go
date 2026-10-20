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

func foregroundBlocked(pid int, table processTable) bool {
	root, ok := table[pid]
	if !ok {
		return false
	}
	if root.state == 'R' {
		return false
	}
	children := make(map[int][]int, len(table))
	for child, state := range table {
		children[state.ppid] = append(children[state.ppid], child)
	}
	seen := map[int]bool{pid: true}
	pending := []int{pid}
	leaves := 0
	for len(pending) > 0 {
		member := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		state := table[member]
		if state.state == 'R' {
			return false
		}
		if len(children[member]) == 0 {
			leaves++
			if state.state != 'S' {
				return false
			}
		}
		for _, child := range children[member] {
			// Process snapshots can contain cycles; visit each PID only once.
			if !seen[child] {
				seen[child] = true
				pending = append(pending, child)
			}
		}
	}
	return leaves > 0
}
