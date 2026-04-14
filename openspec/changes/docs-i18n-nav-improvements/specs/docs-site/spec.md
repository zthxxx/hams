# Docs Site — i18n & Navigation Improvements (Delta Spec)

## MODIFIED Requirements

### Requirement: Landing Page (MODIFIED)

The site SHALL have a dedicated landing page that introduces hams. The landing page SHALL use a full-width raw layout without sidebar or table-of-contents.

#### Scenario: Landing page locale routes

WHEN a user visits `hams.zthxxx.me/`
THEN the site SHALL render the English landing page by default
AND `/en` SHALL return an HTTP 302 redirect to `/`
AND `/zh-CN` SHALL render the Simplified Chinese landing page.

#### Scenario: Landing page content (unchanged)

WHEN a user visits `hams.zthxxx.me/`
THEN the page SHALL display:
- A hero section with the project name, tagline, and a brief description of what hams does
- A call-to-action linking to the documentation (`/en/docs/`)
- Key feature highlights
AND the page SHALL use Nextra's `theme: { layout: 'raw' }` to render without sidebar/TOC.

#### Scenario: Landing page navigation (unchanged)

WHEN the landing page is displayed
THEN the landing page entry SHALL be configured with `display: 'hidden'` in `_meta.ts` so it does not appear in the top navigation bar.

### Requirement: Docs Section Routing (ADDED)

All documentation pages SHALL live under locale-prefixed paths. The English docs SHALL be at `/en/docs/...` and Chinese docs at `/zh-CN/docs/...`. A bare `/docs` path SHALL redirect to the default locale.

#### Scenario: English docs under /en/docs

WHEN a user navigates to `/en/docs`
THEN the page SHALL display the "Why hams?" content as the default docs landing
AND the left sidebar SHALL be visible with all documentation sections
AND the URL SHALL remain `/en/docs` (no further redirect).

#### Scenario: Chinese docs under /zh-CN/docs

WHEN a user navigates to `/zh-CN/docs`
THEN the page SHALL display the Chinese "为什么选择 hams？" content as the default docs landing
AND the left sidebar SHALL be visible with Chinese navigation labels
AND the URL SHALL remain `/zh-CN/docs` (no further redirect).

#### Scenario: Bare /docs redirect

WHEN a user navigates to `/docs` without a locale prefix
THEN the site SHALL return an HTTP 302 redirect to `/en/docs`.

#### Scenario: Sidebar navigation to docs sub-pages

WHEN a user clicks a sidebar section (e.g., "CLI Reference" → "hams apply")
THEN the URL SHALL be `/{locale}/docs/{section}/{page}` (e.g., `/en/docs/cli/apply` or `/zh-CN/docs/cli/apply`)
AND the sidebar SHALL highlight the current page.

### Requirement: Top Navigation Bar (MODIFIED)

#### Scenario: Top bar items (MODIFIED)

WHEN a user views any page on the site
THEN the top navigation bar SHALL contain, in order:
1. Project name/logo (links to `/`)
2. "Documentation" (links to `/en/docs/` when in English context, `/zh-CN/docs/` when in Chinese context)
3. Search input (Nextra built-in Flexsearch)
4. GitHub icon (links to repository)
5. Language switcher icon (multilingual icon with dropdown, at the far right)
AND the language switcher SHALL be the rightmost element in the navbar
AND the "Documentation" link SHALL use the current locale prefix.

### Requirement: Explicit Top-Level Navigation (MODIFIED)

#### Scenario: Top-level navigation entries (MODIFIED)

WHEN a user views the site
THEN all top-level navigation entries SHALL be explicitly declared in `pages/_meta.ts` with `type: 'page'`
AND navigation structure MUST NOT rely on implicit file-path-based conventions
AND adding or removing a top-level section SHALL require an explicit edit to the `_meta.ts` file
AND the `_meta.ts` file SHALL define display names for all entries.

### Requirement: i18n Support (MODIFIED)

The documentation site SHALL support internationalization with English (`en`) as the default language and Simplified Chinese (`zh-CN`) as the first additional locale. The i18n system SHALL be designed for easy extension to additional locales.

#### Scenario: English locale routing

WHEN a user visits `hams.zthxxx.me/` without a locale prefix
THEN the landing page SHALL render in English
AND `/en` SHALL 302 redirect to `/`
AND English documentation pages SHALL be served at `/en/docs/...`.

#### Scenario: Chinese locale routing

WHEN a user visits `hams.zthxxx.me/zh-CN`
THEN the Simplified Chinese landing page SHALL render
AND Chinese documentation pages SHALL be served at `/zh-CN/docs/...`.

#### Scenario: Locale switcher (MODIFIED — multilingual icon)

WHEN a user views the top navigation bar
THEN a multilingual icon SHALL be displayed at the far right of the top navigation bar
AND the icon SHALL be sourced from a Nextra-compatible icon library (e.g., `react-icons`) and SHALL use an icon specifically designed to represent multilingual/language switching (e.g., a "translate" or "language" icon resembling "En/中" or similar glyph — NOT a generic globe)
AND clicking the icon SHALL reveal a select/dropdown listing available languages (e.g., "English", "简体中文")
AND selecting a language SHALL navigate to the equivalent page in the selected locale, preserving the current document path (e.g., `/en/docs/cli/apply` → `/zh-CN/docs/cli/apply`)
AND the switcher SHALL NOT navigate to the locale's root/landing page when switching from a documentation page
AND the `zh-CN` entry in root `_meta.ts` SHALL use `display: 'hidden'` so it does not appear as a top navigation tab.

#### Scenario: Docs section default page

WHEN a user navigates to `/{locale}/docs` (e.g., `/en/docs` or `/zh-CN/docs`)
THEN the page SHALL display the first chapter ("Why hams?" / "为什么选择 hams？") content by default
AND the left sidebar SHALL be visible with all documentation sections
AND clicking the "Why hams?" sidebar item SHALL navigate to `/{locale}/docs/why-hams` with the same content displayed.

#### Scenario: i18n file structure (directory-based without Nextra i18n)

WHEN a developer adds a new documentation page
THEN the i18n structure SHALL use a directory-based convention WITHOUT Nextra's built-in i18n (which is incompatible with `output: 'export'`):
- English content in `pages/en/<path>.mdx` (under `/en/` locale directory)
- Chinese content in `pages/zh-CN/<path>.mdx` (under `/zh-CN/` locale directory)
AND each locale directory SHALL have its own `_meta.ts` file with localized navigation labels
AND `next.config.mjs` SHALL NOT use the `i18n` config key (incompatible with static export).

#### Scenario: i18n content synchronization (unchanged)

WHEN any documentation content is added, modified, or removed in any locale
THEN ALL other locale versions of that content SHALL be updated in the same change
AND the CI pipeline SHALL verify that every English page has a corresponding Chinese page (and vice versa).

### Requirement: Navigation Structure (MODIFIED)

#### Scenario: CLI section default expanded

WHEN a user navigates to any page in the documentation
THEN the CLI Reference section in the sidebar SHALL be expanded by default, showing all subcommand pages
AND other collapsible sections SHALL follow the default collapse behavior defined by `defaultMenuCollapseLevel`.

#### Scenario: Sidebar navigation (unchanged)

WHEN a user views any page on the site
THEN the left sidebar SHALL display the chapter hierarchy:
1. Why hams?
2. Quickstart / Install
3. CLI Reference (with nested subcommand pages, expanded by default)
4. Builtin Provider Catalog (with nested per-provider pages)
5. Schema Reference (with nested per-schema pages)
6. Provider API
AND the current page SHALL be visually highlighted in the sidebar.
