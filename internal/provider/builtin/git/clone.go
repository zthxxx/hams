package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zthxxx/hams/internal/config"
	hamserr "github.com/zthxxx/hams/internal/error"
	"github.com/zthxxx/hams/internal/hamsfile"
	"github.com/zthxxx/hams/internal/provider"
	"github.com/zthxxx/hams/internal/state"
)

// CloneProvider implements the git clone filesystem provider.
type CloneProvider struct {
	cfg *config.Config
}

// NewCloneProvider creates a new git clone provider.
func NewCloneProvider(cfg *config.Config) *CloneProvider { return &CloneProvider{cfg: cfg} }

// Manifest returns the git clone provider metadata.
func (p *CloneProvider) Manifest() provider.Manifest {
	return provider.Manifest{
		Name:          "git-clone",
		DisplayName:   "git clone",
		Platforms:     []provider.Platform{provider.PlatformAll},
		ResourceClass: provider.ClassFilesystem,
		FilePrefix:    "git-clone",
	}
}

// Bootstrap checks if git is available.
func (p *CloneProvider) Bootstrap(_ context.Context) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH")
	}
	return nil
}

// Probe checks if the local path exists for each tracked clone.
func (p *CloneProvider) Probe(_ context.Context, sf *state.File) ([]provider.ProbeResult, error) {
	var results []provider.ProbeResult
	for id, r := range sf.Resources {
		if r.State == state.StateRemoved {
			continue
		}

		// For git clone, the resource ID format is "remote -> local-path".
		localPath := extractLocalPath(id)
		if localPath == "" {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed, ErrorMsg: "invalid format"})
			continue
		}

		// Expand leading `~/` so hamsfiles shared across machines work
		// out-of-the-box — each user's $HOME is resolved per-invocation
		// rather than hard-coded into the YAML. Without this, a hamsfile
		// with `path: ~/repos/foo` would make os.Stat check a literal
		// `~/repos/foo` directory (typically non-existent), flagging
		// every tracked clone as StateFailed on machines where the user
		// didn't explicitly materialize a `~` subdirectory.
		if expanded, expErr := config.ExpandHome(localPath); expErr == nil {
			localPath = expanded
		}

		// A bare path-exists check is not enough: if the user removes
		// `.git/` (or the bare-repo HEAD file) but leaves sibling files,
		// Stat still succeeds, Probe said StateOK, and the next apply
		// would skip the resource despite the clone being semantically
		// broken. Require the same markers that ensureStoreIsGitRepo
		// uses at the CLI layer: either `.git` (non-bare repo) or
		// `HEAD` (bare repo). Absence of both flips the resource to
		// StateFailed so the next apply re-clones.
		if !isGitRepoPath(localPath) {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateFailed})
		} else {
			results = append(results, provider.ProbeResult{ID: id, State: state.StateOK})
		}
	}
	return results, nil
}

// isGitRepoPath reports whether path contains either a `.git` entry
// (non-bare repo) or a `HEAD` file (bare repo). Distinguishes
// "directory exists AND is still a git repo" from "directory exists
// but no longer a repo" — the latter used to probe as StateOK,
// masking drift that a subsequent apply would silently skip.
func isGitRepoPath(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err == nil {
		return true
	}
	return false
}

// cloneResource holds parsed fields from a git-clone hamsfile entry.
type cloneResource struct {
	Remote string
	Path   string
	Branch string
}

// Plan computes actions for git clone entries, parsing structured YAML fields.
func (p *CloneProvider) Plan(_ context.Context, desired *hamsfile.File, observed *state.File) ([]provider.Action, error) {
	resourceByID, err := cloneParseResources(desired)
	if err != nil {
		return nil, err
	}

	// Build app list from parsed resources for ComputePlan.
	var apps []string
	for id := range resourceByID {
		apps = append(apps, id)
	}

	actions := provider.ComputePlan(apps, observed, observed.ConfigHash)
	for i := range actions {
		res, ok := resourceByID[actions[i].ID]
		if ok {
			actions[i].Resource = res
		}
	}
	return actions, nil
}

// Apply clones a repository to the specified path.
func (p *CloneProvider) Apply(ctx context.Context, action provider.Action) error {
	var remote, localPath, branch string

	// Try structured resource first, then fall back to parsing the ID.
	if res, ok := action.Resource.(cloneResource); ok {
		remote = res.Remote
		localPath = res.Path
		branch = res.Branch
	} else {
		remote, localPath = parseCloneResource(action.ID)
	}

	if remote == "" || localPath == "" {
		return fmt.Errorf("git-clone: resource must have remote and path")
	}

	// Expand `~/` in the destination so git clone targets the real
	// home directory instead of creating a literal `~` subdirectory
	// in CWD. The hamsfile intentionally keeps the unexpanded form so
	// it remains portable across machines (see Probe for the same
	// expansion on the read-side).
	if expanded, expErr := config.ExpandHome(localPath); expErr == nil {
		localPath = expanded
	}

	// Guard the "dir exists but isn't a git repo" case that Probe
	// now flags as StateFailed (cycle 135) — without this check,
	// ComputePlan promotes StateFailed → ActionInstall → git clone
	// fails with "destination path X already exists and is not an
	// empty directory", leaving the user with a cryptic error. Surface
	// an actionable UserFacingError instead so the user knows exactly
	// what to do.
	if info, statErr := os.Stat(localPath); statErr == nil && info.IsDir() {
		if !isGitRepoPath(localPath) {
			return hamserr.NewUserError(hamserr.ExitGeneralError,
				fmt.Sprintf("git-clone target %q already exists but is not a git repository", localPath),
				"Either delete the directory and re-run apply: rm -rf "+localPath,
				"Or initialize it in place: cd "+localPath+" && git init && git remote add origin "+remote,
			)
		}
		// Already a git repo at this path — skip clone (idempotent).
		slog.Info("git-clone: target already exists as a git repo, skipping clone", "path", localPath)
		return nil
	}

	slog.Info("git clone", "remote", remote, "path", localPath)
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, remote, localPath)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Remove is a no-op — we don't delete cloned repos.
func (p *CloneProvider) Remove(_ context.Context, resourceID string) error {
	slog.Warn("git-clone: remove is a no-op (won't delete directories)", "resource", resourceID)
	return nil
}

// List returns cloned repos with status.
func (p *CloneProvider) List(_ context.Context, _ *hamsfile.File, sf *state.File) (string, error) {
	// Sort IDs before iteration so `hams git-clone list` produces stable
	// alphabetical row order across invocations. Without the sort, Go's
	// non-deterministic map iteration shuffled the rows on every call —
	// breaks grep/diff/snapshot workflows over the output. Symmetric
	// with cycles 148-153.
	ids := make([]string, 0, len(sf.Resources))
	for id := range sf.Resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var sb strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&sb, "  %-60s %s\n", id, sf.Resources[id].State)
	}
	return sb.String(), nil
}

// HandleCommand processes CLI subcommands for git clone.
func (p *CloneProvider) HandleCommand(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	verb, remaining := provider.ParseVerb(args)

	switch verb {
	case "add":
		return p.handleAdd(ctx, remaining, hamsFlags, flags)
	case "remove":
		return p.handleRemove(remaining, hamsFlags, flags)
	case "list":
		return p.handleList(ctx, hamsFlags, flags)
	default:
		// Passthrough: treat as raw git clone.
		if len(args) < 2 {
			return hamserr.NewUserError(hamserr.ExitUsageError,
				"git-clone requires a subcommand or remote URL and local path",
				"Usage: hams git-clone add <remote> --hams-path=<path>",
				"       hams git-clone remove <urn-id>",
				"       hams git-clone list",
			)
		}
		return p.clonePassthrough(ctx, args, flags)
	}
}

func (p *CloneProvider) handleAdd(ctx context.Context, args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"git-clone add requires a remote URL",
			"Usage: hams git-clone add <remote> --hams-path=<path>",
		)
	}

	remote := args[0]
	localPath := hamsFlags["path"]
	if localPath == "" {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"git-clone add requires --hams-path",
			"Usage: hams git-clone add <remote> --hams-path=<path>",
		)
	}

	// Expand `~/` for the git clone invocation but keep `localPath`
	// unexpanded for the hamsfile record — the stored form is what
	// ships to another machine, so a literal `~/` there is a feature
	// (each machine's $HOME resolves per-invocation in Apply/Probe).
	cloneTarget := localPath
	if expanded, expErr := config.ExpandHome(localPath); expErr == nil {
		cloneTarget = expanded
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would clone: git clone %s %s\n", remote, cloneTarget)
		return nil
	}

	// Mirror Apply's cycle-136 guard: detect non-git-but-existing dir
	// and already-a-valid-repo cases before shelling out to git.
	// Without this, users of `hams git-clone add` hit the same cryptic
	// "destination already exists" shell error that declarative apply
	// users hit before cycle 136.
	if info, statErr := os.Stat(cloneTarget); statErr == nil && info.IsDir() {
		if !isGitRepoPath(cloneTarget) {
			return hamserr.NewUserError(hamserr.ExitGeneralError,
				fmt.Sprintf("git-clone target %q already exists but is not a git repository", cloneTarget),
				"Either delete the directory and re-run: rm -rf "+cloneTarget,
				"Or initialize it in place: cd "+cloneTarget+" && git init && git remote add origin "+remote,
			)
		}
		// Already a git repo — skip the clone but still record in the
		// hamsfile so the user's explicit `add` intent is captured.
		// Common scenario: user manually cloned the repo earlier, now
		// wants hams to track it.
		slog.Info("git-clone: target already a git repo, recording in hamsfile without cloning", "path", cloneTarget)
		return p.recordAdd(remote, localPath, hamsFlags, flags)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", remote, cloneTarget) //nolint:gosec // git clone from CLI input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	return p.recordAdd(remote, localPath, hamsFlags, flags)
}

// recordAdd persists the just-cloned resource to the hamsfile (with
// the unexpanded path so the YAML is portable across machines) and
// the state file (StateOK). Extracted from handleAdd so the auto-record
// bookkeeping is unit-testable independently of the git clone exec
// call. Failure writing either file is surfaced with a wrapped
// error — a half-recorded state where hamsfile has the entry but
// state doesn't (or vice versa) is worse than reporting the failure
// because the user would then see drift on the next apply without
// understanding why.
func (p *CloneProvider) recordAdd(remote, localPath string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	// Record in hamsfile using "remote -> local-path" as the resource ID.
	resourceID := remote + " -> " + localPath
	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	hf.AddApp("repos", resourceID, "")
	if err := hf.Write(); err != nil {
		return fmt.Errorf("git-clone: failed to write hamsfile: %w", err)
	}

	// Mirror CP-1 auto-record: write state as well so `hams list`
	// immediately reflects the new resource without requiring a
	// separate `hams refresh`. Same contract satisfied by apt,
	// homebrew, git-config, defaults, duti, and the 7 Package-class
	// providers covered by the 2026-04-16-package-provider-auto-record-gap
	// change.
	sf := p.loadOrCreateStateFile(flags)
	sf.SetResource(resourceID, state.StateOK)
	if saveErr := sf.Save(p.statePath(flags)); saveErr != nil {
		return fmt.Errorf("git-clone: failed to write state: %w", saveErr)
	}

	slog.Info("git-clone: cloned and recorded", "remote", remote, "path", localPath)
	return nil
}

func (p *CloneProvider) handleRemove(args []string, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	if len(args) == 0 {
		return hamserr.NewUserError(hamserr.ExitUsageError,
			"git-clone remove requires a resource ID",
			"Usage: hams git-clone remove <urn-id>",
		)
	}

	resourceID := args[0]
	if flags.DryRun {
		fmt.Printf("[dry-run] Would remove entry: %s (directory NOT deleted)\n", resourceID)
		return nil
	}

	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	// Validate the ID exists in hamsfile OR state before mutating
	// either. Without this check, a typo like `hams git-clone remove
	// git@github.com:foo/br` (missing `ar`) would silently succeed:
	// RemoveApp no-ops (returns false, ignored) and the state file
	// gets a useless StateRemoved tombstone for a nonexistent ID.
	// Surface a clear error with the list of tracked IDs so the user
	// can retry with a valid one.
	inHamsfile := slices.Contains(hf.ListApps(), resourceID)
	_, inState := sf.Resources[resourceID]
	if !inHamsfile && !inState {
		tracked := append([]string(nil), hf.ListApps()...)
		for id := range sf.Resources {
			if !slices.Contains(tracked, id) {
				tracked = append(tracked, id)
			}
		}
		// Sort the tracked-IDs suggestion list so the user sees the same
		// "Tracked IDs: …" text on each typo retry. hf.ListApps preserves
		// hamsfile source order; the state-only IDs from the loop above
		// were appended in non-deterministic map-iteration order. Sorting
		// the merged list gives a single deterministic surface. Symmetric
		// with cycles 148-153.
		sort.Strings(tracked)
		suggestions := []string{"Run 'hams git-clone list' to see tracked repos"}
		if len(tracked) > 0 {
			suggestions = append(suggestions, "Tracked IDs: "+strings.Join(tracked, ", "))
		}
		return hamserr.NewUserError(hamserr.ExitUsageError,
			fmt.Sprintf("git-clone: no tracked resource with ID %q", resourceID),
			suggestions...,
		)
	}

	hf.RemoveApp(resourceID)
	if err := hf.Write(); err != nil {
		return fmt.Errorf("git-clone: failed to write hamsfile: %w", err)
	}

	// Mirror git-config's doRemove: mark the state resource as
	// StateRemoved so `hams list` / `hams refresh` see a tombstone
	// rather than a stale StateOK that would look orphaned (or worse
	// re-clone on next apply). Matches the auto-record contract other
	// CLI-writing providers (apt, homebrew, git-config, defaults,
	// duti) satisfy for their remove paths.
	sf.SetResource(resourceID, state.StateRemoved)
	if err := sf.Save(p.statePath(flags)); err != nil {
		return fmt.Errorf("git-clone: failed to write state: %w", err)
	}

	slog.Warn("git-clone: entry removed from Hamsfile. Local directory was NOT deleted.", "resource", resourceID)
	return nil
}

// handleList loads the hamsfile + state and prints the enumerated
// managed repositories. Previously `case "list":` printed only the
// header — users ran the command and saw nothing below it, giving
// no hint whether state existed or was empty. Now: header + either
// the tracked repositories (id, state) or an actionable empty-state
// hint pointing at `git-clone add`.
func (p *CloneProvider) handleList(ctx context.Context, hamsFlags map[string]string, flags *provider.GlobalFlags) error {
	hf, err := p.loadOrCreateHamsfile(hamsFlags, flags)
	if err != nil {
		return err
	}
	sf := p.loadOrCreateStateFile(flags)

	output, err := p.List(ctx, hf, sf)
	if err != nil {
		return err
	}

	fmt.Println("git clone managed repositories:")
	if output == "" {
		fmt.Println("  (no clones tracked yet — add one with: hams git-clone add <remote> --hams-path=<path>)")
		return nil
	}
	fmt.Print(output)
	return nil
}

func (p *CloneProvider) clonePassthrough(ctx context.Context, args []string, flags *provider.GlobalFlags) error {
	remote := args[0]
	localPath := args[1]

	// Expand `~/` for parity with the recorded add path and with
	// Apply/Probe — without this, `hams git-clone <remote> "~/repos/foo"`
	// would create a literal `~` subdirectory in CWD rather than
	// cloning under $HOME.
	if expanded, expErr := config.ExpandHome(localPath); expErr == nil {
		localPath = expanded
	}

	if flags.DryRun {
		fmt.Printf("[dry-run] Would clone: git clone %s %s\n", remote, localPath)
		return nil
	}

	cmd := exec.CommandContext(ctx, "git", "clone", remote, localPath) //nolint:gosec // git clone from CLI input
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Name returns the CLI name.
func (p *CloneProvider) Name() string { return "git-clone" }

// DisplayName returns the display name.
func (p *CloneProvider) DisplayName() string { return "git clone" }

func (p *CloneProvider) loadOrCreateHamsfile(hamsFlags map[string]string, flags *provider.GlobalFlags) (*hamsfile.File, error) {
	path, err := p.hamsfilePath(hamsFlags, flags)
	if err != nil {
		return nil, err
	}
	return hamsfile.LoadOrCreateEmpty(path)
}

func (p *CloneProvider) hamsfilePath(hamsFlags map[string]string, flags *provider.GlobalFlags) (string, error) {
	cfg := p.effectiveConfig(flags)
	if cfg.StorePath == "" {
		return "", hamserr.NewUserError(hamserr.ExitUsageError,
			"no store directory configured",
			"Set store_path in hams config or pass --store",
		)
	}

	suffix := ".hams.yaml"
	if _, ok := hamsFlags["local"]; ok {
		suffix = ".hams.local.yaml"
	}

	return filepath.Join(cfg.ProfileDir(), p.Manifest().FilePrefix+suffix), nil
}

// statePath returns the absolute path to git-clone's state file for
// the currently active machine under the active profile. Mirrors
// ConfigProvider.statePath from hamsfile.go.
func (p *CloneProvider) statePath(flags *provider.GlobalFlags) string {
	cfg := p.effectiveConfig(flags)
	return filepath.Join(cfg.StateDir(), p.Manifest().FilePrefix+".state.yaml")
}

// loadOrCreateStateFile reads git-clone's state file for the active
// machine, returning a fresh empty document when the file does not
// yet exist OR is unreadable. Fail-open matches ConfigProvider's
// behavior: the next Save() will replace a corrupted file rather
// than blocking the user on a disk/permission issue unrelated to
// what they're trying to do.
func (p *CloneProvider) loadOrCreateStateFile(flags *provider.GlobalFlags) *state.File {
	cfg := p.effectiveConfig(flags)
	sf, err := state.Load(p.statePath(flags))
	if err != nil {
		return state.New(p.Manifest().Name, cfg.MachineID)
	}
	return sf
}

func (p *CloneProvider) effectiveConfig(flags *provider.GlobalFlags) *config.Config {
	if p.cfg == nil {
		p.cfg = &config.Config{}
	}
	cfg := *p.cfg
	if flags.Store != "" {
		cfg.StorePath = flags.Store
	}
	return &cfg
}

func extractLocalPath(resourceID string) string {
	_, localPath := parseCloneResource(resourceID)
	return localPath
}

// parseCloneResource splits a legacy resource ID of the form "remote -> local-path".
// Branch is intentionally not encoded in the ID — structured YAML fields carry it instead.
func parseCloneResource(id string) (remote, localPath string) {
	parts := strings.SplitN(id, " -> ", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// cloneParseResources parses structured git-clone entries from the hamsfile.
// Supports both structured (remote/path/branch fields) and legacy (urn as "remote -> path") formats.
func cloneParseResources(f *hamsfile.File) (map[string]cloneResource, error) {
	if f.Root == nil || len(f.Root.Content) == 0 {
		return map[string]cloneResource{}, nil
	}

	doc := f.Root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("git-clone provider: hamsfile root must be a mapping")
	}

	resourceByID := make(map[string]cloneResource)
	for i := 1; i < len(doc.Content); i += 2 {
		seq := doc.Content[i]
		if seq.Kind != yaml.SequenceNode {
			continue
		}

		for _, item := range seq.Content {
			if item.Kind == yaml.ScalarNode {
				// Simple string entry: treat as "remote -> path" format.
				id := item.Value
				remote, localPath := parseCloneResource(id)
				if remote != "" && localPath != "" {
					resourceByID[id] = cloneResource{Remote: remote, Path: localPath}
				}
				continue
			}
			if item.Kind != yaml.MappingNode {
				continue
			}

			var id string
			var res cloneResource
			for j := 0; j < len(item.Content)-1; j += 2 {
				key := item.Content[j].Value
				value := item.Content[j+1].Value
				switch key {
				case "urn":
					id = value
				case "app":
					id = value
				case "remote":
					res.Remote = value
				case "path", "local_path":
					res.Path = value
				case "branch", "default_branch":
					res.Branch = value
				}
			}

			if id == "" {
				continue
			}

			// If structured fields present, use them. Otherwise try to parse ID.
			if res.Remote == "" || res.Path == "" {
				remote, localPath := parseCloneResource(id)
				if remote != "" {
					res.Remote = remote
				}
				if localPath != "" {
					res.Path = localPath
				}
			}

			resourceByID[id] = res
		}
	}

	return resourceByID, nil
}
