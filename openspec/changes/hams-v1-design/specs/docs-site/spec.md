# Docs Site Spec

This spec defines the documentation site for hams, built with Nextra and deployed to GitHub Pages at `hams.zthxxx.me`. The site serves as the primary reference for users, contributors, and provider authors.

## Context

hams is a declarative IaC environment management tool for macOS/Linux workstations. The documentation site is an independent capability (see design.md dependency graph) written after other specs are stable. It does not depend on the Go codebase at build time but references all other specs for content accuracy.

The site lives in the `docs/` directory at the repository root and uses the Nextra documentation framework (React/Next.js). All code examples are hand-crafted and designed -- never auto-generated from source code.

## ADDED Requirements

### Requirement: Nextra Framework Setup

The documentation site SHALL use Nextra as the static site generation framework, located in the `docs/` directory of the repository root.

#### Scenario: Initialize Nextra project

WHEN a developer clones the repository and runs the docs setup command
THEN the `docs/` directory SHALL contain a valid Nextra project with:
- `package.json` with `nextra` and `nextra-theme-docs` as dependencies
- `next.config.mjs` configured with the Nextra plugin
- A `theme.config.tsx` (or `.jsx`) file configuring the Nextra docs theme
- A `pages/` directory containing the documentation content as `.mdx` files
- A `tsconfig.json` for TypeScript support

#### Scenario: Local development server

WHEN a developer runs the local development command from `docs/`
THEN the site SHALL start a local Next.js development server with hot-reload on port 3000 (or next available)
AND all MDX content changes SHALL reflect in the browser without a full rebuild.

#### Scenario: Static export for deployment

WHEN the build command is executed in `docs/`
THEN Nextra SHALL produce a fully static HTML export in `docs/out/`
AND the export SHALL contain no server-side runtime dependencies
AND the exported site SHALL be servable from any static file host.

### Requirement: GitHub Pages Deployment

The site SHALL be deployed to GitHub Pages at `hams.zthxxx.me` via GitHub Actions CI/CD. The root URL (`hams.zthxxx.me`) SHALL serve a homepage/landing page. The documentation site SHALL be accessible at `hams.zthxxx.me/docs`.

#### Scenario: Automated deployment on main branch push

WHEN a commit is pushed to the `main` branch that modifies files under `docs/`
THEN a GitHub Actions workflow SHALL:
1. Install dependencies and build the static export
2. Deploy the contents of `docs/out/` to GitHub Pages
AND the site SHALL be accessible at `https://hams.zthxxx.me` within 5 minutes of the workflow completing.

#### Scenario: Custom domain configuration

WHEN the site is deployed to GitHub Pages
THEN a `CNAME` file containing `hams.zthxxx.me` SHALL exist in the `docs/public/` directory
AND the GitHub repository settings SHALL have the custom domain configured
AND HTTPS SHALL be enforced for all requests.

#### Scenario: Pull request preview skips deployment

WHEN a pull request modifies files under `docs/`
THEN the CI workflow SHALL build the site to verify it compiles without errors
BUT SHALL NOT deploy to GitHub Pages.

### Requirement: Chapter Structure -- Why / Motivation

The site SHALL include a "Why hams?" chapter as the primary entry point for understanding the project's purpose.

#### Scenario: Comparison table

WHEN a user navigates to the "Why hams?" page
THEN the page SHALL display a comparison table covering at minimum:
- NixOS / nix-darwin flakes
- Terraform
- Pulumi
- Ansible
- chezmoi
- brew bundle (Brewfile)
AND each row SHALL describe why that tool is insufficient for hams' use case
AND the comparison SHALL be factually accurate and fair to each tool.

#### Scenario: What hams is NOT section

WHEN a user reads the "Why hams?" page
THEN the page SHALL explain the name origin: "hams" is short for hamster, a creature that loves to hoard things — just like this tool hoards your environment configurations for safekeeping and restoration.
AND the page SHALL include a detailed comparison vs Ansible highlighting:
- hams adds explicit idempotency checks (`check:` fields) on top of Ansible's implicit module idempotency
- hams adds a single-source-of-truth state file to avoid re-running all checks
- hams enables "install via CLI now, auto-record for later" workflow (vs Ansible's "write playbook first" approach)
AND the page SHALL contain an explicit "What hams is NOT" section that clearly states:
- hams is NOT a Docker/CI replacement
- hams is NOT NixOS-level isolation (no sandboxing, no hermetic builds)
- hams is NOT a project-level tool (it is host-level)
- hams is NOT a dotfile manager (though it can manage dotfiles via the `file` provider)

### Requirement: Chapter Structure -- Quickstart / Install

The site SHALL include a Quickstart chapter that guides new users from zero to a working hams installation with their first apply.

#### Scenario: Installation methods

WHEN a user navigates to the Quickstart page
THEN the page SHALL document at minimum three installation methods:
1. `curl | bash` one-liner: `bash -c "$(curl -fsSL https://github.com/zthxxx/hams/raw/master/install.sh)"`
2. Homebrew tap: `brew install zthxxx/tap/hams`
3. Binary download from GitHub Releases
AND each method SHALL include a complete, copy-pasteable command.

#### Scenario: First apply walkthrough

WHEN a user follows the Quickstart guide after installing hams
THEN the page SHALL walk through a complete `hams apply --from-repo=<user/repo>` flow including:
- What happens when no local profile exists (profile tag + machine-id prompts)
- What the sudo prompt means and when it appears
- Expected terminal output (as a styled code block, not a screenshot)
- How to verify the apply succeeded

### Requirement: Chapter Structure -- CLI Reference

The site SHALL include a CLI Reference chapter documenting every hams subcommand.

#### Scenario: Subcommand documentation completeness

WHEN a user navigates to the CLI Reference chapter
THEN every hams subcommand SHALL have its own section or page covering:
- Command syntax with all flags and arguments
- Description of what the command does
- At least one usage example with expected behavior
- Flag table listing each flag, its type, default value, and description
AND the following subcommands SHALL be documented at minimum:
- `hams apply`
- `hams refresh`
- `hams self-upgrade`
- `hams config`
- `hams <provider> install` (generic pattern + provider-specific examples)
- `hams <provider> remove`
- `hams <provider> list`
- `hams <provider> enrich`

#### Scenario: Global flags documentation

WHEN a user views any CLI Reference page
THEN a "Global Flags" section SHALL be accessible (either on each page or as a shared reference) documenting:
- `--debug`
- `--only=<providers>`
- `--except=<providers>`
- `--help`
- `--` separator semantics
AND the `--hams:` prefix convention for provider-scoped flags SHALL be explained.

### Requirement: Chapter Structure -- Builtin Provider Catalog

The site SHALL include a Builtin Provider Catalog chapter with a dedicated page for each builtin provider.

#### Scenario: Per-provider page content

WHEN a user navigates to a specific builtin provider page (e.g., Homebrew)
THEN the page SHALL contain:
- Provider display name and description
- Supported platforms
- Store schema: annotated example of the provider's `<Provider>.hams.yaml` format with all supported fields
- Available commands (install, remove, list, enrich) with examples
- Provider-specific flags (using `--hams:` prefix)
- At least one complete, realistic example Hamsfile snippet
AND the example YAML SHALL be valid and parseable.

#### Scenario: Provider catalog index

WHEN a user navigates to the Builtin Provider Catalog index page
THEN a table or card layout SHALL list all builtin providers with:
- Provider name
- Wrapped tool (e.g., "Homebrew" wraps `brew`)
- Supported platforms (macOS, Linux, or both)
- Brief one-line description
AND the list SHALL include at minimum: Bash, Homebrew, apt, pnpm, npm, uv, go, cargo, VSCode Extension, git (config + clone), defaults, duti, mas.

### Requirement: Chapter Structure -- Schema Reference

The site SHALL include a Schema Reference chapter documenting all YAML file formats used by hams.

#### Scenario: hams.config.yaml documentation

WHEN a user navigates to the Schema Reference for `hams.config.yaml`
THEN the page SHALL include:
- A fully annotated example of `hams.config.yaml` with inline YAML comments explaining each field
- Documentation for global config (`${HAMS_CONFIG_HOME}/hams.config.yaml`) vs project-level config
- Documentation for `.local.yaml` merge semantics
- All configurable fields including: profile tag, machine-id, LLM CLI path, store repo path, notification channels, provider-priority list

#### Scenario: Provider.hams.yaml documentation

WHEN a user navigates to the Schema Reference for Hamsfile format
THEN the page SHALL include:
- The general structure of `<Provider>.hams.yaml` files
- How `.local.yaml` merging works (append vs override semantics)
- The role of tags, intro, hooks (pre/post-install, defer)
- URN structure for script-type providers (`urn:hams:<provider>:<resource-id>`)
- At least two annotated examples: one for a package-type provider (e.g., Homebrew) and one for a script-type provider (e.g., Bash)

#### Scenario: Provider.state.yaml documentation

WHEN a user navigates to the Schema Reference for state files
THEN the page SHALL include:
- The structure of `<Provider>.state.yaml`
- Per-resource status values: `ok`, `failed`, `pending`, `removed`
- Fields per resource class (package, kv-config, check-based, filesystem)
- An annotated example state file
- A note that state files are machine-generated and MUST NOT be hand-edited

### Requirement: Chapter Structure -- Provider API

The site SHALL include a Provider API chapter for users who want to write custom providers.

#### Scenario: Provider authoring guide

WHEN a user navigates to the Provider API chapter
THEN the page SHALL document:
- The provider interface contract (Register, Bootstrap, Probe, Plan, Apply, Remove, List, Enrich lifecycle)
- How to use the Go SDK (`pkg/sdk`) to implement a provider
- The `hashicorp/go-plugin` extension mechanism for external providers
- How to declare `depend-on` relationships and platform-conditional support
- The manifest format for provider metadata
- At least one complete example of a minimal custom provider implementation in Go

#### Scenario: Provider resource classes

WHEN a user reads the Provider API chapter
THEN the page SHALL explain the four resource classes and their probe contracts:
1. Package (native list command)
2. KV Config (read-back command)
3. Check-based (user-supplied `check:` command)
4. Filesystem (path existence)
AND each class SHALL include the required state fields and an example.

### Requirement: i18n Support

The documentation site SHALL support internationalization with English as the primary language and Chinese as the first additional locale.

#### Scenario: English as default locale

WHEN a user visits `hams.zthxxx.me` without a locale prefix
THEN the site SHALL render in English
AND the URL structure SHALL NOT include a `/en/` prefix for English pages.

#### Scenario: Chinese locale availability

WHEN a user switches to Chinese via the Nextra locale switcher
THEN Chinese-translated pages SHALL be served where available
AND pages without Chinese translations SHALL fall back to English content
AND the locale switcher SHALL be visible in the site navigation.

#### Scenario: i18n file structure

WHEN a developer adds a new documentation page
THEN the i18n structure SHALL follow Nextra conventions:
- English content in `pages/<path>.mdx` (or `pages/<path>/index.mdx`)
- Chinese content in `pages/<path>.zh-CN.mdx` (or equivalent Nextra i18n pattern)
AND the `next.config.mjs` SHALL configure `i18n` with locales `['en', 'zh-CN']` and `defaultLocale: 'en'`.

### Requirement: Built-in Search

The documentation site SHALL provide full-text search using Nextra's built-in Flexsearch integration.

#### Scenario: Search functionality

WHEN a user types a query into the search bar
THEN Flexsearch SHALL return matching pages from all documentation chapters
AND results SHALL display page titles and content excerpts
AND search SHALL work entirely client-side with no external service dependency.

#### Scenario: Search across locales

WHEN a user searches while viewing the Chinese locale
THEN the search SHALL prioritize Chinese content where available
AND fall back to English content for untranslated pages.

### Requirement: Visual Design

The documentation site SHALL have a clean, developer-focused visual design with dark mode as the default.

#### Scenario: Dark mode default

WHEN a user visits `hams.zthxxx.me` for the first time
THEN the site SHALL render in dark mode by default
AND a theme toggle SHALL be available to switch between dark, light, and system-preference modes
AND the toggle SHALL persist the user's preference via local storage.

#### Scenario: Code block styling

WHEN a documentation page contains code examples
THEN code blocks SHALL have syntax highlighting appropriate to the language (YAML, Go, Bash, etc.)
AND code blocks SHALL include a copy-to-clipboard button
AND multi-line code blocks SHALL display with line numbers where appropriate.

#### Scenario: Mobile responsiveness

WHEN a user visits the site on a mobile device
THEN the layout SHALL be responsive with a collapsible sidebar navigation
AND all content SHALL be readable without horizontal scrolling.

### Requirement: Code Example Quality

All code examples in the documentation SHALL be hand-crafted, designed, and valid.

#### Scenario: YAML example validity

WHEN a documentation page contains a YAML code example
THEN the YAML SHALL be syntactically valid and parseable by a standard YAML parser
AND the example SHALL use realistic values (real package names, plausible tags, sensible hook commands)
AND the example SHALL demonstrate the documented feature accurately.

#### Scenario: Go code example validity

WHEN a documentation page contains a Go code example (e.g., in the Provider API chapter)
THEN the Go code SHALL be syntactically valid
AND the code SHALL use correct import paths (`github.com/zthxxx/hams/...`)
AND the code SHALL follow the project's code conventions (import grouping, naming).

#### Scenario: Bash example validity

WHEN a documentation page contains a Bash code example
THEN the Bash command SHALL be a valid, executable command
AND dangerous or destructive commands SHALL include a warning callout
AND placeholder values SHALL use a consistent convention (e.g., `<your-repo>`, `<provider-name>`).

### Requirement: Navigation Structure

The site navigation SHALL reflect the chapter structure and provide clear wayfinding.

#### Scenario: Sidebar navigation

WHEN a user views any page on the site
THEN the left sidebar SHALL display the chapter hierarchy:
1. Why hams?
2. Quickstart / Install
3. CLI Reference (with nested subcommand pages)
4. Builtin Provider Catalog (with nested per-provider pages)
5. Schema Reference (with nested per-schema pages)
6. Provider API
AND the current page SHALL be visually highlighted in the sidebar.

#### Scenario: Breadcrumb navigation

WHEN a user is on a nested page (e.g., CLI Reference > hams apply)
THEN breadcrumb navigation SHALL indicate the page's position in the hierarchy.

#### Scenario: Previous/Next page links

WHEN a user finishes reading a page
THEN "Previous" and "Next" links at the bottom of the page SHALL guide sequential reading through the documentation in logical order.

### Requirement: Site Metadata and SEO

The documentation site SHALL include proper metadata for search engine discoverability.

#### Scenario: Page metadata

WHEN a search engine crawls the site
THEN each page SHALL have:
- A unique `<title>` tag incorporating the page name and "hams"
- A `<meta name="description">` tag summarizing the page content
- Open Graph tags for social media sharing
AND the site SHALL include a `robots.txt` allowing full crawling
AND the site SHALL generate a `sitemap.xml`.

#### Scenario: Favicon and branding

WHEN a user opens the site in a browser tab
THEN a favicon SHALL be displayed
AND the site header SHALL display the hams logo/wordmark and link to the repository on GitHub.
