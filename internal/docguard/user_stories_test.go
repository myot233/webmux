package docguard

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

var (
	storyIDPattern    = regexp.MustCompile(`US-([A-Z])([0-9]{2})`)
	storyPattern      = regexp.MustCompile(`^- \*\*US-([A-Z])([0-9]{2})`)
	storyRangePattern = regexp.MustCompile(`US-([A-Z])([0-9]{2})–US-([A-Z])([0-9]{2})`)
)

type storyID struct {
	group string
	n     int
}

func (id storyID) String() string { return fmt.Sprintf("US-%s%02d", id.group, id.n) }

func TestUserStories(t *testing.T) {
	violations, err := checkUserStories(os.DirFS(filepath.Join("..", "..")))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Error(report(violations))
	}
}

func checkUserStories(fsys fs.FS) ([]violation, error) {
	stories := map[storyID]string{}
	retired := map[storyID]bool{}
	var out []violation
	err := fs.WalkDir(fsys, "docs/user-stories", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path.Ext(name) != ".md" || isStoryHistory(name) {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			match := storyPattern.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			n, _ := strconv.Atoi(match[2])
			id := storyID{match[1], n}
			if previous, exists := stories[id]; exists {
				out = append(out, violation{name, "story-id", fmt.Sprintf("%s 与 %s 重复；故事 ID 必须唯一", id, previous)})
			} else {
				stories[id] = name
			}
			if strings.Contains(line, "已退役") {
				retired[id] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, name := range []string{"docs/user-stories.md", "docs/user-stories/coverage.md"} {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		text := string(data)
		for _, match := range storyIDPattern.FindAllStringSubmatch(text, -1) {
			n, _ := strconv.Atoi(match[2])
			id := storyID{match[1], n}
			if _, ok := stories[id]; !ok {
				out = append(out, violation{name, "story-ref", fmt.Sprintf("%s 没有对应故事", id)})
			}
		}
		for _, match := range storyRangePattern.FindAllStringSubmatch(text, -1) {
			first, _ := strconv.Atoi(match[2])
			last, _ := strconv.Atoi(match[4])
			if match[1] != match[3] || last < first {
				out = append(out, violation{name, "story-range", fmt.Sprintf("无效范围 %s", match[0])})
				continue
			}
			for n := first; n <= last; n++ {
				id := storyID{match[1], n}
				if _, ok := stories[id]; !ok {
					out = append(out, violation{name, "story-range", fmt.Sprintf("范围 %s 缺少 %s", match[0], id)})
				}
			}
		}
	}

	for id, name := range stories {
		if !retired[id] {
			continue
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if storyPattern.MatchString(line) {
				continue
			}
			if strings.Contains(line, id.String()) && !strings.Contains(line, "退役") {
				out = append(out, violation{name, "story-retired", fmt.Sprintf("%s 已退役，不能在未说明历史关系的行中重新引用", id)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path || out[i].msg < out[j].msg })
	return out, nil
}

func isStoryHistory(name string) bool {
	base := path.Base(name)
	return base == "coverage.md" || strings.HasPrefix(base, "verification-log-")
}

func TestUserStoriesRules(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/user-stories.md":          &fstest.MapFile{Data: []byte("| A | US-A01–US-A02 |\n| B | US-B01–US-B02 |\n| US-A99 |\n")},
		"docs/user-stories/coverage.md": &fstest.MapFile{Data: []byte("| US-A01–US-A02 |\n")},
		"docs/user-stories/a.md":        &fstest.MapFile{Data: []byte("- **US-A01 现役**\n- **US-A02 历史（已退役）**\n- **US-A02 重复**\n")},
	}
	violations, err := checkUserStories(fsys)
	if err != nil {
		t.Fatal(err)
	}
	text := report(violations)
	for _, want := range []string{"重复", "US-A99", "缺少", "US-B01"} {
		if !strings.Contains(text, want) {
			t.Fatalf("story guard output lacks %q:\n%s", want, text)
		}
	}
}
