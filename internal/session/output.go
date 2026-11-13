package session

// OutputPage is a bounded slice of retained raw PTY bytes. It is independent of
// rendering recovery and never attaches a client, redraws or claims control.
type OutputPage struct {
	Generation                       string
	Start, End, NextOffset           uint64
	Data                             []byte
	Reset, HistoryTruncated, Limited bool
}

func (s *Session) ReadOutput(resume *ResumePoint, limit int) (OutputPage, error) {
	if limit < 1 || limit > 65536 {
		return OutputPage{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		return OutputPage{}, ErrNoRun
	}
	r := s.run.ring
	r.mu.RLock()
	defer r.mu.RUnlock()
	oldest, end := r.total-uint64(r.size), r.total
	start := oldest
	reset := false
	if resume != nil {
		reset = resume.Generation != s.info.Generation || resume.Offset < oldest || resume.Offset > end
		if !reset {
			start = resume.Offset
		}
	}
	n := min(uint64(limit), end-start)
	data := make([]byte, int(n))
	first := (r.next - r.size + int(start-oldest) + len(r.data)) % len(r.data)
	copied := copy(data, r.data[first:min(first+int(n), len(r.data))])
	copy(data[copied:], r.data[:int(n)-copied])
	return OutputPage{Generation: s.info.Generation, Start: start, End: end, NextOffset: start + n, Data: data, Reset: reset, HistoryTruncated: oldest > 0, Limited: start+n < end}, nil
}
