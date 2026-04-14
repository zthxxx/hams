# Docs Site Verification Process

## When to Run

Run this verification process whenever docs site content, configuration, or dependencies change.

## Execution Flow

### 1. Install & Start Dev Server

In background task run:

```bash
pnpm install
cd docs && pnpm dev
```

Wait for the dev server to respond with HTTP 200 at the basePath URL.

### 2. Verify with Playwright (Dev Server)

Use `playwright-cli` skill to verify each page category. For each page:
- First take a **snapshot** (DOM structure) to verify content and links
- Then take a **screenshot** to verify visual rendering and layout when needed

**Pages to verify** (at minimum):
1. Homepage — content, sidebar, footer, dark mode toggle, GitHub link
2. Quickstart — installation steps, code blocks
3. CLI Reference index — command table with working links
4. At least one provider page — content, breadcrumbs
5. Any locale variants if i18n is enabled

**Link verification**: Confirm no double-prefix URLs (e.g., `/docs/docs/...`). With `basePath: '/docs'`, all internal MDX links must omit the `/docs` prefix — Nextra adds it automatically.

### 3. Record Issues

Write all findings to `.claude/docs-site-issues.md`:
- Categorize as CRITICAL / MINOR
- Include affected files and line numbers
- Mark issues as "Fixed" once resolved

### 4. Fix Issues via Subagent

If issues are found:
1. Keep the dev server running
2. Spawn a subagent to fix each issue
3. Re-verify in the browser after each fix

### 5. Production Build

```bash
cd docs && rm -rf .next && pnpm build
```

- All pages must generate without errors
- Check `docs/out/` exists with expected HTML files

### 6. Verify Build Output

Serve the build output with the correct basePath structure:
```bash
mkdir -p /tmp/deploy/docs
cp -r docs/out/* /tmp/deploy/docs/
cd /tmp/deploy && python3 -m http.server 3940
```

Use `playwright-cli` to verify at `http://localhost:3940/docs/`:
- Pages render with correct assets (CSS/JS load)
- Internal links navigate correctly
- No broken references

### 7. Verify Deploy Path Alignment

Confirm the build output structure matches the GitHub Actions workflow:
- `docs/out/` files go under `deploy/docs/` directory (matching `basePath: '/docs'`)
- `CNAME` file at deploy root
- `peaceiris/actions-gh-pages@v4` publishes to `gh-pages` branch

### Screenshot Conventions

Save all screenshots to `.playwright-cli/` (already in `.gitignore`):
- Format: `<scenario>-<validation-intent>-<YYYYMMDDTHHmmss>.png`
- Example: `homepage-full-render-20260414T070814.png`
- Build verification prefix: `build-<scenario>-...`

### Playwright CLI Guidelines

- When using `playwright-cli`, always capture a snapshot before taking a screenshot.
- Save all screenshots in the project's `.playwright-cli/` directory.
- Use screenshot filenames in the format `<scenario>-<validation-intent>-<YYYYMMDDTHHmmss>.png`.
- Use the same filename convention for visual comparison screenshots.
