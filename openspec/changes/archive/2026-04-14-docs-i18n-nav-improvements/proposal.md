# Docs i18n & Navigation Improvements

## What Changes

Improve the documentation site's i18n architecture, routing structure, and language switching UX:

1. **Locale-prefixed docs routing**: All documentation pages live under locale-prefixed paths (`/en/docs/...`, `/zh-CN/docs/...`). English content moves from root `/docs/` to `/en/docs/`. Bare `/docs` 302-redirects to `/en/docs`.
2. **Landing page routing**: `/` serves English landing page (default). `/en` 302-redirects to `/`. `/zh-CN` serves Chinese landing page.
3. **Top nav "Documentation" button**: Links to `/{current-locale}/docs` based on current language context.
4. **Language switcher UX**: Replace the globe SVG with a proper multilingual icon from a Nextra-compatible icon library (e.g., `react-icons` translate/language icon resembling "En/中"). Click reveals dropdown. Switching language preserves the current document path.
5. **CLI section expanded by default**: The CLI Reference sidebar section SHALL be expanded by default, showing all subcommand pages without requiring user click.
6. **Docs section default page**: `/{locale}/docs` defaults to showing the first chapter ("Why hams?") with sidebar — no redirect.
7. **i18n content sync**: Any documentation content change must synchronize all locale versions in the same commit.

## Why

- Current routing has English docs at root `/docs/` without locale prefix, creating asymmetry with `/zh-CN/docs/`. Moving to `/en/docs/` makes the structure consistent and extensible.
- `/docs` without locale prefix needs a redirect so bookmarks and external links still work.
- Globe icon is too generic for language switching — a dedicated multilingual icon (like "translate" glyph) better communicates the purpose.
- CLI is the most-referenced section; expanding it by default reduces clicks for the common case.
- Language switching must preserve document path — users reading a specific page shouldn't be dumped to the landing page.

## Impact

- `docs-site` spec: Landing page routing, docs routing, i18n scenarios, top nav, sidebar behavior all updated.
- File structure: English pages move from `pages/docs/` to `pages/en/docs/`. Root `pages/` keeps only landing page and redirect stubs.
- Existing external links to `/docs/...` will 302-redirect to `/en/docs/...`.
