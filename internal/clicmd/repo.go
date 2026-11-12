package clicmd

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// maxInitInstruction mirrors the server's limit on an initialization
// instruction, in characters.
const maxInitInstruction = 4000

// builtinReferences are the instruction variables that are not quick
// commands.
var builtinReferences = []string{"user", "dir", "webmux"}

var instructionReference = regexp.MustCompile(`\{\{([^{}\n]+)\}\}`)

var repoGroup = &Command{
	Name: "repo", Group: GroupWorkspace,
	Summary: "Register repositories and read or change their settings, branches and commits.",
}

var repoListCommand = &Command{
	Name: "repo ls", Group: GroupWorkspace,
	Summary: "List the registered repositories.",
	Output: "A table: ID, NAME, PATH, AVAILABILITY, WORKTREES, HOME (worktrees the home page shows), INIT (whether an " +
		"initialization instruction is set). With --json, the Repository array.",
	Exit: exitsCommon(),
	Examples: []Example{
		{Command: "webmux repo ls", Text: "See which repositories WebMux knows."},
		{Command: "webmux repo ls --json | jq -r '.[].anchorPath'", Text: "List their paths."},
	},
	SeeAlso: []string{"webmux repo add", "webmux worktree ls"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the Repository array")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 0 {
				return e.Usagef("repo ls takes no arguments")
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			list, err := repositories(ctx, client)
			if err != nil {
				return e.Fail(err)
			}
			if *asJSON {
				if list == nil {
					list = []contract.Repository{}
				}
				writeJSON(e.Stdout, list)
				return contract.ExitOK
			}
			h, err := loadHome(ctx, client)
			if err != nil {
				return e.Fail(err)
			}
			w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "ID\tNAME\tPATH\tAVAILABILITY\tWORKTREES\tHOME\tINIT")
			for _, r := range list {
				shown := 0
				for _, t := range r.Worktrees {
					if h.status(r, t) != homeHidden {
						shown++
					}
				}
				init := "-"
				if r.InitInstruction != "" {
					init = "yes"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n", r.ID, r.Name, r.AnchorPath, r.Availability, len(r.Worktrees), shown, init)
			}
			_ = w.Flush()
			return contract.ExitOK
		}
	},
}

var repoAddCommand = &Command{
	Name: "repo add", Group: GroupWorkspace, Args: "[PATH]",
	Summary:   "Register a Git repository (default: the current directory); an already registered one is printed as is.",
	Arguments: []Item{{Name: "PATH", Text: "A directory in the repository; the current directory when omitted."}},
	Output:    "\"ID<TAB>NAME<TAB>PATH\"; with --json, the Repository.",
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "Also: PATH is not a Git repository (the server's reason is printed)."}),
	Examples: []Example{
		{Command: "webmux repo add", Text: "Register the repository you are in."},
		{Command: "git clone https://example.com/app.git && webmux repo add app", Text: "Register a fresh clone."},
	},
	SeeAlso: []string{"webmux repo settings", "webmux repo rm"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the Repository")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) > 1 {
				return e.Usagef("at most one path is allowed")
			}
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			dir, err := e.localPath(ctx, arg)
			if err != nil {
				return e.Fail(err)
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			before, err := repositories(ctx, client)
			if err != nil {
				return e.Fail(err)
			}
			var repo contract.Repository
			if err := client.Do(ctx, http.MethodPost, "/api/repositories", contract.RegisterRepositoryRequest{Path: dir}, &repo); err != nil {
				return e.Fail(err)
			}
			if slices.ContainsFunc(before, func(r contract.Repository) bool { return r.ID == repo.ID }) {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: already registered")
			}
			if *asJSON {
				writeJSON(e.Stdout, repo)
			} else {
				_, _ = fmt.Fprintf(e.Stdout, "%s\t%s\t%s\n", repo.ID, repo.Name, repo.AnchorPath)
			}
			return contract.ExitOK
		}
	},
}

var repoRemoveCommand = &Command{
	Name: "repo rm", Group: GroupWorkspace, Args: "REPO",
	Summary: "Unregister a repository; nothing on disk is deleted.",
	Description: "The repository disappears from every browser's workspace and its home page settings are cleared. Its " +
		"worktrees, branches and files stay. On a terminal the command asks first; without a terminal it needs --yes.",
	Arguments: []Item{repoArgument},
	Exit:      exitsCommon(ExitItem{Code: 2, Text: "Also: no terminal to confirm and no --yes, or REPO names nothing."}),
	Examples: []Example{
		{Command: "webmux repo rm .", Text: "Stop tracking the current repository."},
		{Command: "webmux repo rm abcd2345 --yes", Text: "Unregister one in a script."},
	},
	SeeAlso: []string{"webmux repo add", "webmux repo ls"},
	Bind: func(fs *flag.FlagSet) Action {
		yes := fs.Bool("yes", false, "do not ask for confirmation")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one repository is required")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			if !*yes {
				if code, ok := e.confirm(fmt.Sprintf("Unregister %s (%s)? Files stay on disk.", repo.Name, repo.AnchorPath)); !ok {
					return code
				}
			}
			if err := client.Do(ctx, http.MethodDelete, "/api/repositories/"+url.PathEscape(repo.ID), nil, nil); err != nil {
				return e.Fail(err)
			}
			if err := updateHome(ctx, client, repo, func([]string, bool) ([]string, bool) { return nil, false }); err != nil {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: unregistered, but its home page setting was not cleared:", err)
			}
			return contract.ExitOK
		}
	},
}

var repoShowCommand = &Command{
	Name: "repo show", Group: GroupWorkspace, Args: "REPO",
	Summary: "Show a repository: paths, availability, last fetch, initialization instruction and worktrees with their " +
		"upstream, changes, last commit and home page state.",
	Arguments: []Item{repoArgument},
	Output: "Aligned key-value lines and a worktree table; last fetch is the newest FETCH_HEAD time, approximate. With " +
		"--json, the Repository with home states.",
	Exit: exitsCommon(ExitItem{Code: 2, Text: "Also: REPO names nothing."}),
	Examples: []Example{
		{Command: "webmux repo show .", Text: "Inspect the current repository."},
		{Command: "webmux repo show . --json | jq .initInstruction", Text: "Read its initialization instruction."},
	},
	SeeAlso: []string{"webmux repo settings", "webmux worktree ls"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the Repository with home states")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one repository is required")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			return e.showRepository(ctx, client, repo, *asJSON)
		}
	},
}

func (e Env) showRepository(ctx context.Context, client *localclient.Client, repo contract.Repository, asJSON bool) int {
	h, err := loadHome(ctx, client)
	if err != nil {
		return e.Fail(err)
	}
	view := h.view(repo)
	if asJSON {
		writeJSON(e.Stdout, view)
		return contract.ExitOK
	}
	init := "-"
	if repo.InitInstruction != "" {
		init = fmt.Sprintf("%s (%d characters)", oneLine(repo.InitInstruction, 60), utf8.RuneCountInString(repo.InitInstruction))
	}
	fetched := "-"
	if repo.LastFetchedAt != nil {
		fetched = repo.LastFetchedAt.Local().Format(time.DateTime) + " (approximate)"
	}
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	for _, row := range [][2]string{
		{"id", repo.ID}, {"name", repo.Name}, {"path", repo.AnchorPath}, {"git dir", repo.CommonDir},
		{"availability", repo.Availability}, {"error", repo.Error}, {"checked", repo.CheckedAt.Local().Format(time.DateTime)},
		{"last fetch", fetched}, {"init", init},
	} {
		if row[1] != "" {
			_, _ = fmt.Fprintf(w, "%s\t%s\n", row[0], row[1])
		}
	}
	_ = w.Flush()
	_, _ = fmt.Fprintln(e.Stdout)
	w = tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tKIND\tBRANCH\tPATH\tSTATE\tSESSIONS\tHOME\tUPSTREAM\tCHANGES\tLAST COMMIT")
	for _, t := range view.Worktrees {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n", t.ID, t.Kind, worktreeName(t.Worktree), t.Path, worktreeState(t.Worktree),
			t.Sessions, t.Home, upstreamName(t.Worktree), changeCounts(t.Changes), commitLine(t.LastCommit))
	}
	_ = w.Flush()
	return contract.ExitOK
}

var repoRefreshCommand = &Command{
	Name: "repo refresh", Group: GroupWorkspace, Args: "REPO",
	Summary:     "Check a repository again and print its current worktrees, like the Web UI's refresh button.",
	Description: "Use it after changing worktrees outside WebMux (git worktree add or remove), so every browser shows them.",
	Arguments:   []Item{repoArgument},
	Output:      "The repository as repo show prints it; with --json, the Repository with home states.",
	Exit:        exitsCommon(ExitItem{Code: 2, Text: "Also: REPO names nothing."}),
	Examples: []Example{
		{Command: "git worktree add ../app-hotfix hotfix && webmux repo refresh .", Text: "Pick up a worktree made with git."},
		{Command: "webmux repo refresh . --json | jq '.worktrees | length'", Text: "Count the worktrees after checking."},
	},
	SeeAlso: []string{"webmux repo show", "webmux worktree ls"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the Repository with home states")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one repository is required")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			var fresh contract.Repository
			if err := client.Do(ctx, http.MethodGet, "/api/repositories/"+url.PathEscape(repo.ID)+"/worktrees", nil, &fresh); err != nil {
				return e.Fail(err)
			}
			return e.showRepository(ctx, client, fresh, *asJSON)
		}
	},
}

var repoSettingsCommand = &Command{
	Name: "repo settings", Group: GroupWorkspace, Args: "REPO",
	Summary: "Show or change a repository's settings (the Web UI's repository settings dialog).",
	Description: "The only setting is the initialization instruction, which quick start (and webmux run --init) puts before the " +
		"first message. Without options, the instruction is printed with its length, the limit and the references it uses. " +
		"--init sets it from TEXT, - (standard input) or @FILE; text that is empty after trimming clears it, as " +
		"--clear-init does. --edit opens it in $VISUAL or $EDITOR.\n\n" +
		"The instruction may use {{webmux}} (the WebMux tool brief), {{dir}} (the session's artifacts folder), {{user}} " +
		"(where the user's message goes) and {{name}} for a saved quick command. A reference to a quick command that does " +
		"not exist is reported as a warning and saved anyway; quick start refuses it while it is missing.",
	Arguments: []Item{repoArgument},
	Output:    "The instruction and its references; with --json, {initInstruction}.",
	Exit: exitsCommon(
		ExitItem{Code: 2, Text: fmt.Sprintf("Also: the instruction is longer than %d characters, or --edit has no terminal.", maxInitInstruction)},
	),
	Examples: []Example{
		{Command: "printf '{{webmux}}\\n\\n{{user}}' | webmux repo settings . --init -", Text: "Give new agents the WebMux tool brief."},
		{Command: "webmux repo settings . --init @.webmux/init.md", Text: "Set the instruction from a file."},
		{Command: "webmux repo settings .", Text: "Show the instruction and what it references."},
	},
	SeeAlso: []string{"webmux run", "webmux guide workspace"},
	Notes: map[string]Note{
		"init":       {Conflicts: []string{"clear-init", "edit"}},
		"clear-init": {Conflicts: []string{"init", "edit"}},
		"edit":       {Conflicts: []string{"init", "clear-init"}},
	},
	Bind: func(fs *flag.FlagSet) Action {
		init := fs.String("init", "", "set the initialization instruction to `TEXT`, - for standard input, @FILE for a file")
		clearInit := fs.Bool("clear-init", false, "clear the initialization instruction")
		edit := fs.Bool("edit", false, "edit the instruction in $VISUAL or $EDITOR")
		asJSON := fs.Bool("json", false, "print {initInstruction}")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one repository is required")
			}
			initSet := false
			fs.Visit(func(f *flag.Flag) { initSet = initSet || f.Name == "init" })
			if countTrue(initSet, *clearInit, *edit) > 1 {
				return e.Usagef("give only one of --init, --clear-init and --edit")
			}
			client, repo, code := e.repoAndClient(ctx, args[0])
			if client == nil {
				return code
			}
			defer client.Close()
			text, change := repo.InitInstruction, true
			switch {
			case initSet:
				value, err := e.readValue(*init)
				if err != nil {
					return e.Fail(err)
				}
				text = value
			case *clearInit:
				text = ""
			case *edit:
				edited, code, ok := e.editText(repo.InitInstruction)
				if !ok {
					return code
				}
				text = edited
			default:
				change = false
			}
			text = strings.TrimSpace(text)
			if n := utf8.RuneCountInString(text); n > maxInitInstruction {
				return e.Usagef("the instruction has %d characters; at most %d are allowed", n, maxInitInstruction)
			}
			if change {
				if err := client.Do(ctx, http.MethodPatch, "/api/repositories/"+url.PathEscape(repo.ID),
					contract.UpdateRepositoryRequest{InitInstruction: text}, &repo); err != nil {
					return e.Fail(err)
				}
			}
			commands, missing := e.instructionReferences(ctx, client, repo.InitInstruction)
			if len(missing) > 0 {
				refs := make([]string, len(missing))
				for i, name := range missing {
					refs[i] = "{{" + name + "}}"
				}
				_, _ = fmt.Fprintf(e.Stderr, "webmux: no quick command named %s; saved anyway, but quick start refuses it until it exists\n", strings.Join(refs, ", "))
			}
			if *asJSON {
				writeJSON(e.Stdout, map[string]string{"initInstruction": repo.InitInstruction})
				return contract.ExitOK
			}
			if change {
				return contract.ExitOK
			}
			if repo.InitInstruction == "" {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: no initialization instruction is set")
				return contract.ExitOK
			}
			_, _ = fmt.Fprintln(e.Stdout, repo.InitInstruction)
			_, _ = fmt.Fprintf(e.Stderr, "\n%d of %d characters", utf8.RuneCountInString(repo.InitInstruction), maxInitInstruction)
			if len(commands) > 0 {
				_, _ = fmt.Fprintf(e.Stderr, "; references: %s", strings.Join(commands, ", "))
			}
			_, _ = fmt.Fprintln(e.Stderr)
			return contract.ExitOK
		}
	},
}

// readValue reads an option value: - is standard input, @FILE a file.
func (e Env) readValue(value string) (string, error) {
	switch {
	case value == "-":
		return e.readStdin()
	case strings.HasPrefix(value, "@"):
		data, err := os.ReadFile(value[1:])
		return string(data), err
	}
	return value, nil
}

// instructionReferences lists the references of text in order and the
// quick command names among them that no saved command has.
func (e Env) instructionReferences(ctx context.Context, client *localclient.Client, text string) ([]string, []string) {
	var snapshot contract.SettingsSnapshot
	known := map[string]bool{}
	if client.Do(ctx, http.MethodGet, "/api/settings", nil, &snapshot) == nil && snapshot.Values.QuickCommands != nil {
		for _, c := range *snapshot.Values.QuickCommands {
			known[strings.TrimSpace(c.Name)] = true
		}
	}
	var refs, missing []string
	for _, m := range instructionReference.FindAllStringSubmatch(text, -1) {
		name := strings.TrimSpace(m[1])
		ref := "{{" + name + "}}"
		if slices.Contains(refs, ref) {
			continue
		}
		refs = append(refs, ref)
		if !slices.Contains(builtinReferences, name) && !known[name] {
			missing = append(missing, name)
		}
	}
	return refs, missing
}

// editText opens text in $VISUAL or $EDITOR on the terminal and returns the
// saved result.
func (e Env) editText(text string) (string, int, bool) {
	if e.Interactive == nil || !e.Interactive() {
		return "", e.Usagef("--edit needs a terminal"), false
	}
	editor := ""
	if e.Getenv != nil {
		editor = e.Getenv("VISUAL")
		if editor == "" {
			editor = e.Getenv("EDITOR")
		}
	}
	if editor == "" {
		editor = "vi"
	}
	f, err := os.CreateTemp("", "webmux-init-*.md")
	if err != nil {
		return "", e.Fail(err), false
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return "", e.Fail(err), false
	}
	_ = f.Close()
	// The editor setting may carry arguments ("code --wait"); the file is
	// passed as the last argument, never through a shell.
	argv := append(strings.Fields(editor), f.Name())
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", e.Fail(fmt.Errorf("editor %s: %w", argv[0], err)), false
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return "", e.Fail(err), false
	}
	return string(data), contract.ExitOK, true
}
