# Docs Site Issues Found (2026-04-14)

## Fixed

### CRITICAL: Nextra 3.3 Breaking Changes

1. **`_meta.json` no longer supported** - Nextra 3.3+ requires `_meta.ts` or `_meta.js`.
   - Converted all `_meta.json` files to `_meta.ts`.

2. **`useNextSeoProps` removed** - Removed from `theme.config.tsx`.

3. **`primaryHue` removed** - Removed from `theme.config.tsx`.

4. **Missing `_app.tsx`** - Created `docs/pages/_app.tsx` (Nextra v3 requirement).

5. **`next export` deprecated** - Changed build script to `next build` only.

### CRITICAL: Double `/docs/` Link Prefix

All internal links had hardcoded `/docs/` prefix but `basePath: '/docs'` already adds it.
Fixed all ~30 links across 5 MDX files; then converted all to relative paths for i18n compatibility.

### i18n Infrastructure

- Configured `next.config.mjs` with `i18n: { locales: ['en', 'zh-CN'], defaultLocale: 'en' }`
- Added locale switcher to `theme.config.tsx`
- Restructured: English content at `pages/en/`, Chinese at `pages/zh-CN/`
- Created 28 zh-CN MDX files (6 fully translated, 22 placeholder with translation notice)
- All internal links use relative paths for locale-independence

## Known Issues (non-blocking)

### MINOR: Missing favicon
No `favicon.ico` in `docs/public/`. Console 404 error. Cosmetic only.

### MINOR: React hydration warnings
~9 hydration mismatch warnings in console during static export hydration. Common with Nextra static export. Pages render correctly after hydration recovery.

## Configuration Added

| File | Purpose |
|------|---------|
| `pnpm-workspace.yaml` | Declares `docs/` as a pnpm workspace package |
| `.github/workflows/docs.yml` | Deploys docs to `gh-pages` on push to `main`/`dev` or merged PRs |
| `.claude/rules/docs-verification.md` | Docs verification workflow for agents |

## Verification Summary

| Check | en (English) | zh-CN (Chinese) |
|-------|:---:|:---:|
| Pages render | PASS | PASS |
| Sidebar navigation | PASS | PASS |
| Locale switcher | PASS | PASS |
| CSS/assets load | PASS | PASS |
| Translation callout on placeholders | N/A | PASS |
| Build succeeds (64 pages) | PASS | PASS |
| Deploy path alignment | PASS | PASS |
