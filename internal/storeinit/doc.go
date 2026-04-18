// Package storeinit scaffolds a fresh hams store directory on first run.
//
// "Scaffold" here means: create the directory, initialize a git repo
// (shelling out to `git init` when it is on PATH, falling back to the
// bundled `go-git` library otherwise), write the embedded templates
// (`.gitignore`, `hams.config.yaml`), and seed the user's machine-scoped
// identity (`profile_tag`, `machine_id`) into the global config so the
// next CLI invocation is silent.
//
// The go-git fallback is required by
// `openspec/specs/project-structure/spec.md` ("The go-git dependency
// SHALL be used as a fallback when system `git` is not available.") and
// exists so `hams apt install htop` on a brand-new container — which
// ships without git on PATH — still succeeds.
//
// The package exposes two DI seams:
//
//   - ExecGitInit — replaces the `exec.Command("git", "init", …)` call.
//     Rebind in tests to avoid spawning a real `git` child.
//   - GoGitInit — replaces `gogit.PlainInit`. Rebind in tests to force
//     the fallback leg without uninstalling `git`.
//
// Both seams default to production behavior; only test code should
// touch them. They are deliberately not hidden behind a constructor
// because the scaffold flow is a one-shot bootstrapping operation —
// dependency-injecting a struct here would be over-engineered.
package storeinit
