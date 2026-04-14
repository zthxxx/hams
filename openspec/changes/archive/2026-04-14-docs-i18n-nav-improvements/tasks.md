# Tasks: Docs i18n & Navigation Improvements

> NOTE: After the initial implementation, the docs site was migrated to Nextra 4 App Router
> + Tailwind 4 (commit `df83ed9`). The task paths below reflect the **final** implementation:
> `content/` instead of `pages/`, `app/layout.tsx` instead of `theme.config.tsx`.

## Phase 1: Initial i18n setup (completed)

- [x] Record initial spec delta
- [x] Hide `zh-CN` from top nav bar (initial approach: `display: 'hidden'` in root `_meta.ts`; later replaced by CSS-hiding, see Phase 2)
- [x] Create `LanguageSwitcher` component (globe icon + dropdown, path-preserving navigation)
- [x] Add `LanguageSwitcher` to the navbar (initial: `theme.config.tsx`; final: `app/layout.tsx` via `Navbar` children)
- [x] Create `/docs/index.mdx` that displays "Why hams?" content (default docs landing)
- [x] Create `/zh-CN/docs/index.mdx` that displays Chinese "Why hams?" content
- [x] Verify dev server: language switcher, path preservation, docs index
- [x] Verify production build passes

## Phase 2: Locale-prefixed routing & UX improvements

- [x] Update spec delta with new routing requirements (landing page, `/en/docs`, redirects, CLI expanded, language icon)
- [x] Move English docs from `pages/docs/` to `pages/en/docs/` (with all `_meta.ts` files) — later migrated to `content/en/docs/` in Nextra 4
- [x] Create `pages/en/_meta.ts` with English locale navigation — later moved to `content/en/_meta.ts`
- [x] Set up redirect: `/en` → `/` (client-side meta refresh + JS redirect via `components/Redirect.tsx`)
- [x] Set up redirect: `/docs` → `/en/docs` (client-side meta refresh + JS redirect via `components/Redirect.tsx`)
- [x] Update root `_meta.ts` — `en` visible as "Documentation" with `href: '/en/docs'`, `zh-CN` visible with CSS-hidden nav link
- [x] Update top nav "Documentation" link to `/en/docs` (via `href` in root `_meta.ts`)
- [x] Update `LanguageSwitcher` to handle `/en/docs/...` ↔ `/zh-CN/docs/...` path mapping
- [x] Replace globe SVG icon with `MdTranslate` from `react-icons/md` (dedicated multilingual icon)
- [x] Set CLI Reference section to expanded by default — initial `theme.config.tsx` was later migrated to `sidebar={{ defaultMenuCollapseLevel: 2, toggleButton: true }}` on the `<Layout>` in `app/layout.tsx`
- [x] Update English landing page CTA link to `/en/docs/why-hams`
- [x] Update Chinese landing page CTA link to `/zh-CN/docs`
- [x] Update `next.config.mjs` — no changes needed for redirects (static export handles them via the Redirect component)
- [x] Verify dev server: all routes, redirects, language switching, CLI expansion
- [x] Verify production build passes

## Phase 3: Nextra 4 App Router + Tailwind 4 migration (completed in commit `df83ed9`)

- [x] Migrate from `pages/` to `content/` directory (Nextra 4 convention)
- [x] Replace `theme.config.tsx` with `app/layout.tsx` composing `Layout`, `Navbar`, `Footer` from `nextra-theme-docs`
- [x] Add `app/[[...mdxPath]]/page.tsx` as the catch-all MDX renderer
- [x] Add `mdx-components.tsx` wiring `useMDXComponents`
- [x] Migrate Tailwind from v3 to v4: `@import "tailwindcss"` in `app/globals.css`, drop `autoprefixer`, add `@tailwindcss/postcss`
- [x] Verify `pnpm build` emits static export to `docs/dist/` with all routes intact
