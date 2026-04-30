# Docs Site — i18n & Navigation Improvements (Delta Spec)

## MODIFIED Requirements

### Requirement: Nextra Framework Setup

The documentation site SHALL use Nextra 4 with the Next.js App Router as the static site generation framework, located in the `docs/` directory of the repository root. Styling SHALL use Tailwind CSS v4 via the `@tailwindcss/postcss` plugin. The legacy `pages/` directory and `theme.config.tsx` configuration file SHALL NOT be used.

#### Scenario: Initialize Nextra 4 project

WHEN a developer clones the repository and runs the docs setup command
THEN the `docs/` directory SHALL contain a valid Nextra 4 App Router project with:
- `package.json` with `nextra@^4`, `nextra-theme-docs@^4`, and `next@^16` as dependencies
- `next.config.mjs` configured via `nextra({ contentDirBasePath, search })` wrapping `output: 'export'`, `distDir: 'dist'`, and `trailingSlash: true`
- `app/layout.tsx` exporting a `RootLayout` that composes `Layout`, `Navbar`, and `Footer` from `nextra-theme-docs`
- `app/[[...mdxPath]]/page.tsx` as the catch-all MDX renderer using `generateStaticParamsFor('mdxPath')` + `importPage` from `nextra/pages`
- `mdx-components.tsx` wiring `useMDXComponents` from `nextra-theme-docs`
- A `content/` directory containing the documentation sources as `.mdx` files (NOT `pages/`)
- `app/globals.css` importing Tailwind v4 via `@import "tailwindcss"` and declaring `@variant dark` for Nextra's `.dark` class
- `postcss.config.js` configured with a single `@tailwindcss/postcss` plugin (no `autoprefixer`)
- A `tsconfig.json` with `jsx: "react-jsx"` for TypeScript + React 19 support

#### Scenario: Local development server

WHEN a developer runs `pnpm dev` from `docs/`
THEN the site SHALL start a local Next.js development server with hot-reload on port 3000 (or next available)
AND all MDX content changes under `content/` SHALL reflect in the browser without a full rebuild.

#### Scenario: Static export for deployment

WHEN `pnpm build` is executed in `docs/`
THEN Nextra SHALL produce a fully static HTML export in `docs/dist/` (not `docs/out/`)
AND the export SHALL contain no server-side runtime dependencies
AND the exported site SHALL be servable from any static file host.

### Requirement: Tailwind CSS

The documentation site SHALL use Tailwind CSS v4 for custom styling of the landing page and any custom components. Configuration SHALL use the CSS-first approach introduced in Tailwind v4 (no `tailwind.config.js` required).

#### Scenario: Tailwind v4 setup

WHEN the docs site is built
THEN Tailwind CSS SHALL be enabled by importing it in `app/globals.css` with `@import "tailwindcss"`
AND PostCSS SHALL use the single `@tailwindcss/postcss` plugin in `postcss.config.js`
AND dark-mode styles SHALL be scoped using `@variant dark (&:where(.dark, .dark *))` to match Nextra's `.dark` class toggle
AND no `tailwind.config.js` file SHALL be required (content sources are auto-detected by the v4 engine).

### Requirement: Landing Page

The site SHALL have a dedicated landing page that introduces hams. The landing page SHALL use Nextra's full-width layout without sidebar, table-of-contents, breadcrumb, or pagination.

#### Scenario: Landing page locale routes

WHEN a user visits `hams.zthxxx.me/`
THEN the site SHALL render the English landing page by default (served from `content/index.mdx`)
AND `/en` SHALL redirect to `/` via a client-side `<Redirect to="/" />` component (meta-refresh + JS `router.replace`)
AND `/zh-CN` SHALL render the Simplified Chinese landing page (served from `content/zh-CN/index.mdx`).

#### Scenario: Landing page content

WHEN a user visits `hams.zthxxx.me/`
THEN the page SHALL display:
- A hero section with the project name, tagline, and a brief description of what hams does
- A call-to-action linking to `/en/docs/why-hams`
- Key feature highlights
AND the page SHALL render without sidebar/TOC/breadcrumb/pagination via `_meta.ts` entries configured with:
```ts
theme: { layout: 'full', sidebar: false, toc: false, breadcrumb: false, pagination: false }
```

#### Scenario: Landing page navigation

WHEN the landing page is displayed
THEN the root `index` entry in `content/_meta.ts` SHALL be configured with `display: 'hidden'` so it does not appear in the top navigation bar.

### Requirement: Top Navigation Bar

The site top navigation bar SHALL display a consistent set of links across all pages. The navbar SHALL be composed in `app/layout.tsx` using the `Navbar` component from `nextra-theme-docs`, with the `LanguageSwitcher` component inserted as `children` (rendered in the navbar's extra-content slot).

#### Scenario: Top bar items

WHEN a user views any page on the site
THEN the top navigation bar SHALL contain, in order:
1. Project name/logo "Hams 🐹" (passed as `logo` prop, links to `/`)
2. "Documentation" entry (links to `/en/docs`) — declared as an `en` page entry with `href: '/en/docs'` in `content/_meta.ts`
3. A `zh-CN` page entry (links to `/zh-CN/docs`) — visually hidden from the navbar via CSS (see "Locale switcher" scenario) so language switching is driven solely by the `LanguageSwitcher` component
4. Search input (Nextra built-in Pagefind/Flexsearch)
5. GitHub project link (passed as `projectLink="https://github.com/zthxxx/hams"`)
6. Language switcher icon (the `LanguageSwitcher` component, rendered at the far right)
AND the language switcher SHALL be the rightmost element in the navbar.

### Requirement: Explicit Top-Level Navigation

The documentation site SHALL use Nextra's `_meta.ts` files with explicit configuration for all top-level navigation entries. Navigation structure MUST NOT rely on implicit file-path-based conventions.

#### Scenario: Top-level navigation entries

WHEN a user views the site
THEN all top-level navigation entries SHALL be explicitly declared in `content/_meta.ts` with `type: 'page'`
AND navigation structure MUST NOT rely on implicit file-path-based conventions
AND adding or removing a top-level section SHALL require an explicit edit to the `_meta.ts` file
AND the `_meta.ts` file SHALL define display names for all entries.

### Requirement: i18n Support

The documentation site SHALL support internationalization with English (`en`) as the default language and Simplified Chinese (`zh-CN`) as the first additional locale. The i18n system SHALL be designed for easy extension to additional locales.

#### Scenario: English locale routing

WHEN a user visits `hams.zthxxx.me/` without a locale prefix
THEN the landing page SHALL render in English
AND `/en` SHALL redirect to `/` via the `<Redirect>` component
AND English documentation pages SHALL be served at `/en/docs/...`.

#### Scenario: Chinese locale routing

WHEN a user visits `hams.zthxxx.me/zh-CN`
THEN the Simplified Chinese landing page SHALL render
AND Chinese documentation pages SHALL be served at `/zh-CN/docs/...`.

#### Scenario: Locale switcher (multilingual icon + CSS-hidden nav tab)

WHEN a user views the top navigation bar
THEN a multilingual icon SHALL be displayed at the far right of the top navigation bar, rendered by `components/LanguageSwitcher.tsx`
AND the icon SHALL be `MdTranslate` from `react-icons/md` (a dedicated "translate" glyph — NOT a generic globe)
AND clicking the icon SHALL open a dropdown listing available languages ("English", "简体中文")
AND selecting a language SHALL navigate to the equivalent page in the selected locale, preserving the current document path (e.g., `/en/docs/cli/apply` ↔ `/zh-CN/docs/cli/apply`)
AND the switcher SHALL detect the current locale by checking whether `usePathname()` starts with `/zh-CN`, defaulting to `en` otherwise
AND the switcher SHALL NOT navigate to the locale's root/landing page when switching from a documentation page
AND the `zh-CN` entry in `content/_meta.ts` SHALL remain declared as `type: 'page'` with `href: '/zh-CN/docs'` (so the route is reachable) BUT SHALL be visually hidden from the top navbar via a CSS rule in `app/globals.css` targeting `.nextra-nav-container a[href="/zh-CN/docs"] { display: none }` — the `display: 'hidden'` meta key is NOT used, because hiding via `_meta.ts` would also remove the entry from Nextra's route graph.

#### Scenario: Docs section default page

WHEN a user navigates to `/{locale}/docs` (e.g., `/en/docs` or `/zh-CN/docs`)
THEN the page SHALL display the first chapter ("Why hams?" / "为什么选择 hams？") content by default
AND the left sidebar SHALL be visible with all documentation sections
AND the `why-hams` entry in `content/{locale}/docs/_meta.ts` SHALL use `display: 'hidden'` to avoid a duplicate sidebar item next to `index`
AND clicking the "Why hams?" sidebar item (rendered as `index`) SHALL keep the URL at `/{locale}/docs`.

#### Scenario: i18n file structure (directory-based without Nextra i18n)

WHEN a developer adds a new documentation page
THEN the i18n structure SHALL use a directory-based convention under `content/` WITHOUT Nextra's built-in i18n (which is incompatible with `output: 'export'`):
- English content in `content/en/<path>.mdx` (under `/en/` locale directory)
- Chinese content in `content/zh-CN/<path>.mdx` (under `/zh-CN/` locale directory)
AND each locale directory SHALL have its own `_meta.ts` file with localized navigation labels
AND `next.config.mjs` SHALL NOT use the `i18n` config key (incompatible with static export).

#### Scenario: i18n content synchronization

WHEN any documentation content is added, modified, or removed in any locale
THEN ALL other locale versions of that content SHALL be updated in the same change
AND the CI pipeline SHALL verify that every English page has a corresponding Chinese page (and vice versa).

### Requirement: Navigation Structure

The site navigation SHALL reflect the chapter structure and provide clear wayfinding. The sidebar behaviour (collapse levels, toggle button, link highlighting) SHALL be configured through the `sidebar` prop of the Nextra `Layout` component in `app/layout.tsx`, NOT through the deprecated `theme.config.tsx`.

#### Scenario: CLI section default expanded

WHEN a user navigates to any page in the documentation
THEN the CLI Reference section in the sidebar SHALL be expanded by default, showing all subcommand pages without requiring user click
AND this SHALL be achieved by passing `sidebar={{ defaultMenuCollapseLevel: 2, toggleButton: true }}` to `<Layout>` in `app/layout.tsx` — a value of `2` keeps top-level folders (depth 1) expanded, which covers the CLI Reference folder and its sibling sections.

#### Scenario: Sidebar navigation

WHEN a user views any page on the site
THEN the left sidebar SHALL display the chapter hierarchy:
1. Why hams? (rendered as the `index` entry of the docs section)
2. Quickstart / Install
3. CLI Reference (with nested subcommand pages, expanded by default)
4. Builtin Provider Catalog (with nested per-provider pages)
5. Schema Reference (with nested per-schema pages)
6. Provider API
AND the current page SHALL be visually highlighted in the sidebar.

## ADDED Requirements

### Requirement: Docs Section Routing

All documentation pages SHALL live under locale-prefixed paths. The English docs SHALL be at `/en/docs/...` and Chinese docs at `/zh-CN/docs/...`. A bare `/docs` path SHALL redirect to the default locale's docs root.

#### Scenario: English docs under /en/docs

WHEN a user navigates to `/en/docs`
THEN the page SHALL display the "Why hams?" content as the default docs landing (via `content/en/docs/index.mdx` re-exporting `why-hams.mdx`)
AND the left sidebar SHALL be visible with all documentation sections
AND the URL SHALL remain `/en/docs` (no further redirect).

#### Scenario: Chinese docs under /zh-CN/docs

WHEN a user navigates to `/zh-CN/docs`
THEN the page SHALL display the Chinese "为什么选择 hams？" content as the default docs landing (via `content/zh-CN/docs/index.mdx` re-exporting `why-hams.mdx`)
AND the left sidebar SHALL be visible with Chinese navigation labels
AND the URL SHALL remain `/zh-CN/docs` (no further redirect).

#### Scenario: Bare /docs redirect

WHEN a user navigates to `/docs` without a locale prefix
THEN `content/docs/index.mdx` SHALL render a `<Redirect to="/en/docs" />` component that immediately redirects to `/en/docs` via meta-refresh + JS `router.replace`.

#### Scenario: Sidebar navigation to docs sub-pages

WHEN a user clicks a sidebar section (e.g., "CLI Reference" → "hams apply")
THEN the URL SHALL be `/{locale}/docs/{section}/{page}` (e.g., `/en/docs/cli/apply` or `/zh-CN/docs/cli/apply`)
AND the sidebar SHALL highlight the current page.
