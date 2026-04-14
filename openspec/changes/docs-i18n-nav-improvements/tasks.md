# Tasks: Docs i18n & Navigation Improvements

## Phase 1: Initial i18n setup (completed)

- [x] Record initial spec delta
- [x] Hide `zh-CN` from top nav bar (set `display: 'hidden'` in root `_meta.ts`)
- [x] Create `LanguageSwitcher` component (globe icon + dropdown, path-preserving navigation)
- [x] Add `LanguageSwitcher` to `theme.config.tsx` via `navbar.extraContent`
- [x] Create `/docs/index.mdx` that displays "Why hams?" content (default docs landing)
- [x] Create `/zh-CN/docs/index.mdx` that displays Chinese "Why hams?" content
- [x] Verify dev server: language switcher, path preservation, docs index
- [x] Verify production build passes

## Phase 2: Locale-prefixed routing & UX improvements

- [x] Update spec delta with new routing requirements (landing page, `/en/docs`, redirects, CLI expanded, language icon)
- [x] Move English docs from `pages/docs/` to `pages/en/docs/` (with all `_meta.ts` files)
- [x] Create `pages/en/_meta.ts` with English locale navigation
- [x] Set up redirect: `/en` → `/` (client-side meta refresh + JS redirect)
- [x] Set up redirect: `/docs` → `/en/docs` (client-side meta refresh + JS redirect)
- [x] Update root `_meta.ts` — `en` visible as "Documentation" with `href: '/en/docs'`, `zh-CN` visible with CSS-hidden nav link
- [x] Update top nav "Documentation" link to `/en/docs` (via `href` in root `_meta.ts`)
- [x] Update `LanguageSwitcher` to handle `/en/docs/...` ↔ `/zh-CN/docs/...` path mapping
- [x] Replace globe SVG icon with `MdTranslate` from `react-icons/md` (dedicated multilingual icon)
- [x] Set CLI Reference section to expanded by default (`defaultMenuCollapseLevel: Infinity` in theme.config.tsx)
- [x] Update English landing page CTA link to `/en/docs/why-hams`
- [x] Update Chinese landing page CTA link to `/zh-CN/docs`
- [x] Update `next.config.mjs` — no changes needed (static export handles redirects via Redirect component)
- [x] Verify dev server: all routes, redirects, language switching, CLI expansion
- [x] Verify production build passes
