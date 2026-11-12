package hybrid

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/agentprofile/omp"
	"github.com/rainchan/webmux/internal/session"
)

func waitTitle(t *testing.T, s *session.Session, want string) {
	t.Helper()
	waitFor(t, "title "+want, func() bool { return s.Info().Title == want })
}

func TestPromptTitle(t *testing.T) {
	long := strings.Repeat("长", 100)
	for _, tc := range []struct{ give, want string }{
		{"  帮我 修复\n登录页的\t重定向问题 ", "帮我 修复 登录页的 重定向问题"},
		{"a\x1b[31mb\x07", "a[31mb"},
		{" \n\t", ""},
		{long, strings.Repeat("长", 79) + "…"},
		{strings.Repeat("长", 80), strings.Repeat("长", 80)},
	} {
		if got := PromptTitle(tc.give); got != tc.want {
			t.Errorf("PromptTitle(%q) = %q, want %q", tc.give, got, tc.want)
		}
	}
}

func TestFirstPromptTitle(t *testing.T) {
	h := newHarness(t, 0)
	in := h.input(omp.Name, SideACP)
	in.Name = ""
	dir := filepath.Base(resolved(t, in.Cwd))
	s := h.create(in)
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "  帮我 修复\n登录页 ")
	if got := s.Info().Title; got != "帮我 修复 登录页" {
		t.Fatalf("title right after prompt %q, want %q", got, "帮我 修复 登录页")
	}
	if rec := h.record(id); rec.Title != "帮我 修复 登录页" {
		t.Fatalf("record title %q", rec.Title)
	}
	st.until("turn end", hasKind(EventTurnEnd))
	chatTurn(t, h, id, "later")
	if got := s.Info().Title; got != "帮我 修复 登录页" {
		t.Fatalf("second prompt changed the title to %q", got)
	}
	chatTurn(t, h, id, "/title")
	waitTitle(t, s, "fake title")
	// A cleared title falls back to the directory, not the prompt.
	chatTurn(t, h, id, "/untitle")
	waitTitle(t, s, dir)
	chatTurn(t, h, id, "again")
	if got := s.Info().Title; got != dir {
		t.Fatalf("title after clearing %q", got)
	}
}

func TestFirstPromptTitleSkipped(t *testing.T) {
	t.Run("named", func(t *testing.T) {
		h := newHarness(t, 0)
		s := h.create(h.input(omp.Name, SideACP))
		id := s.Info().ID
		chatTurn(t, h, id, "hello")
		chatTurn(t, h, id, "/title")
		if got := s.Info().Title; got != "agent" {
			t.Fatalf("named session title %q", got)
		}
		if rec := h.record(id); rec.Name != "agent" {
			t.Fatalf("record %+v", rec)
		}
	})
	t.Run("attachments only", func(t *testing.T) {
		h := newHarness(t, 0)
		in := h.input(omp.Name, SideACP, "FAKEAGENT_PROMPT_IMAGE=1")
		in.Name = ""
		dir := filepath.Base(resolved(t, in.Cwd))
		s := h.create(in)
		id := s.Info().ID
		st := h.follow(id)
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		if _, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Attachments: []Attachment{{Path: upload(t, "shot.png", pngBytes())}}}); err != nil {
			t.Fatal(err)
		}
		st.until("turn end", hasKind(EventTurnEnd))
		chatTurn(t, h, id, "with text")
		if got := s.Info().Title; got != dir {
			t.Fatalf("title %q, want %q", got, dir)
		}
	})
	t.Run("loaded from the terminal", func(t *testing.T) {
		h := newHarness(t, 0)
		h.fast()
		in := h.input(omp.Name, SideTUI)
		in.Name = ""
		dir := filepath.Base(resolved(t, in.Cwd))
		s := h.create(in)
		id := s.Info().ID
		tuiTurn(t, s, "one")
		h.waitEligible(id)
		h.mustSwitch(id, SideACP)
		chatTurn(t, h, id, "continue")
		if got := s.Info().Title; got != dir {
			t.Fatalf("title %q, want %q", got, dir)
		}
	})
}

// A user-chosen name outlives the agent's titles and a close and resume.
func TestRenamedSessionKeepsNameAcrossResume(t *testing.T) {
	h := newHarness(t, 0)
	in := h.input(omp.Name, SideACP)
	in.Name = ""
	s := h.create(in)
	id := s.Info().ID
	if _, err := h.manager.Rename(id, "我的任务"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.SetName(id, "我的任务"); err != nil {
		t.Fatal(err)
	}
	chatTurn(t, h, id, "/title")
	st := s.Info()
	if st.Title != "我的任务" {
		t.Fatalf("agent title replaced the name: %q", st.Title)
	}
	h.closeSession(id)
	if rec := h.record(id); rec.Name != "我的任务" {
		t.Fatalf("record name %q", rec.Name)
	}
	resumed := h.mustResume(id, ResumeInput{Side: SideACP, Env: in.Env})
	if got := resumed.Info().Title; got != "我的任务" {
		t.Fatalf("title after resume %q", got)
	}
	if err := h.svc.SetName("missing1", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown record: %v", err)
	}
}
