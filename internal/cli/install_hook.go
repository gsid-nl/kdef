package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var installHookCmd = &cobra.Command{
	Use:   "install-hook",
	Short: "Install a git pre-commit hook that runs 'kdef validate'",
	Long: `Install a git pre-commit hook in the current repository that runs
'kdef validate' before each commit, aborting the commit if validation fails.

The hook is written to .git/hooks/pre-commit. If a pre-commit hook already
exists, the command refuses to modify it unless --force (overwrite) or
--append (add kdef check to the existing script) is given.`,
	RunE: runInstallHook,
}

var (
	installHookDir    string
	installHookForce  bool
	installHookAppend bool
)

// Sentinel markers used to detect/replace the kdef block inside an existing hook.
const (
	kdefHookMarkerBegin = "# >>> kdef validate >>>"
	kdefHookMarkerEnd   = "# <<< kdef validate <<<"
)

func init() {
	installHookCmd.Flags().StringVar(&installHookDir, "dir", ".", "project directory (used to locate the git repo)")
	installHookCmd.Flags().BoolVar(&installHookForce, "force", false, "overwrite an existing pre-commit hook")
	installHookCmd.Flags().BoolVar(&installHookAppend, "append", false, "append the kdef check to an existing pre-commit hook")
	installHookCmd.MarkFlagsMutuallyExclusive("force", "append")
}

func runInstallHook(cmd *cobra.Command, args []string) error {
	startDir, err := filepath.Abs(installHookDir)
	if err != nil {
		return fmt.Errorf("resolve dir: %w", err)
	}

	gitDir, err := findGitDir(startDir)
	if err != nil {
		return err
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")
	existing, hookExists, err := readExistingHook(hookPath)
	if err != nil {
		return err
	}

	switch {
	case !hookExists:
		// Fresh install — write the full standalone script.
		if err := os.WriteFile(hookPath, []byte(preCommitHookScript), 0o755); err != nil {
			return fmt.Errorf("write hook: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "installed pre-commit hook at %s\n", hookPath)

	case installHookForce:
		if err := os.WriteFile(hookPath, []byte(preCommitHookScript), 0o755); err != nil {
			return fmt.Errorf("write hook: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "overwrote pre-commit hook at %s\n", hookPath)

	case installHookAppend:
		if strings.Contains(existing, kdefHookMarkerBegin) {
			fmt.Fprintf(cmd.OutOrStdout(), "kdef check already present in %s, nothing to do\n", hookPath)
			return nil
		}
		updated := existing
		if !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += "\n" + kdefHookSnippet
		if err := os.WriteFile(hookPath, []byte(updated), 0o755); err != nil {
			return fmt.Errorf("write hook: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "appended kdef check to existing pre-commit hook at %s\n", hookPath)

	default:
		return fmt.Errorf("pre-commit hook already exists at %s (use --append to add the kdef check, or --force to overwrite)", hookPath)
	}

	return nil
}

// readExistingHook returns the contents of an existing hook, whether it exists, and any stat error.
func readExistingHook(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), true, nil
	}
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read existing hook: %w", err)
}

// findGitDir walks up from start looking for a .git directory or file (for worktrees).
func findGitDir(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate, nil
			}
			// .git may be a file pointing to the real gitdir (submodules, worktrees).
			resolved, err := resolveGitFile(candidate)
			if err != nil {
				return "", fmt.Errorf("resolve .git file: %w", err)
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not inside a git repository (searched upward from %s)", start)
		}
		dir = parent
	}
}

// resolveGitFile reads a .git file of the form "gitdir: <path>" and returns the target.
func resolveGitFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	const prefix = "gitdir:"
	line := string(data)
	// Trim trailing newlines/whitespace and the prefix.
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r' || line[len(line)-1] == ' ') {
		line = line[:len(line)-1]
	}
	if len(line) < len(prefix) || line[:len(prefix)] != prefix {
		return "", fmt.Errorf("unexpected .git file contents: %q", line)
	}
	target := line[len(prefix):]
	for len(target) > 0 && target[0] == ' ' {
		target = target[1:]
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return target, nil
}

// kdefHookSnippet is the portable block that actually runs 'kdef validate'.
// It is appended to existing hooks as-is, and embedded inside the standalone
// script used for fresh installs.
const kdefHookSnippet = `# >>> kdef validate >>>
# Installed by 'kdef install-hook'. Do not edit between these markers.
if command -v kdef >/dev/null 2>&1; then
	_kdef_repo_root=$(git rev-parse --show-toplevel) && (cd "$_kdef_repo_root" && kdef validate) || exit 1
	unset _kdef_repo_root
else
	echo "kdef pre-commit hook: 'kdef' not found in PATH, skipping validation" >&2
fi
# <<< kdef validate <<<
`

const preCommitHookScript = `#!/bin/sh
# kdef pre-commit hook — runs 'kdef validate' before each commit.
set -e

` + kdefHookSnippet
