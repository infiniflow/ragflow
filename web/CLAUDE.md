# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with the RAGFlow frontend (`web/`).

## Project Overview

RAGFlow frontend is a React/TypeScript application built with UmiJS:
- **Components**: shadcn/ui
- **Styling**: Tailwind CSS
- **State**: Zustand
- **Data Fetching**: TanStack Query (React Query)
- **i18n**: react-i18next

## Common Commands

```bash
npm install
npm run dev        # Development server
npm run build      # Production build
npm run lint       # ESLint
npm run test       # Jest tests
```

## Development Conventions

### CSS and Layout Debugging
When fixing CSS/layout issues (especially flex truncation, ellipsis, or element sizing), **always inspect the full parent hierarchy** for `flex-shrink`, `min-width`, and `overflow` constraints before applying fixes like `min-w-0`. Do not repeatedly apply the same fix without verifying the root cause.
- Before editing, explain: (1) the full flex/container hierarchy from the target element up to the nearest non-flex ancestor, (2) what constraint is actually causing the bug, and (3) how the proposed fix addresses that root cause.

### Scope and Boundaries
Respect explicit boundaries from the user. If the user says **"only fix the selected line"** or **"do not touch shared types/files"**, follow that instruction exactly. Do not investigate unrelated errors, modify shared schemas (e.g., `LlmSettingFieldSchema`), or refactor other files without confirmation. If a change outside the described scope seems necessary, ask for permission first.

### Internationalization (i18n)
For translation tasks, add keys **only to the explicitly requested language files** (commonly `src/locales/zh.ts` and `src/locales/en.ts`). Do not auto-propagate changes to all language files unless the user explicitly asks.
- **Style for `en.ts`**: Sentence case — first word capitalized, rest lowercase (e.g., `referenceAnswer: 'Reference answer'`). Proper nouns remain as-is.

### React Component Refactoring
When refactoring or extracting components, **verify layout behavior after each structural change** (especially `flex-1`, conditional rendering, or flex direction changes). Check that existing buttons, alignment, and responsive behavior remain intact. After extraction, verify: (1) all original props and behavior are preserved, (2) layout in parent contexts is identical, and (3) no syntax or type errors were introduced.

### State Management and Data Fetching
For React Query / cache invalidation bugs, **carefully compare query keys across all consuming components and mutation hooks**. Mismatched keys (e.g., with/without `refreshCount`) are a common root cause of stale data or duplicate requests.
- Systematically: (1) list every component/hook that calls `useQuery` for this data, (2) compare their query keys character-for-character, (3) check every mutation's `onSuccess` for cache invalidation, and (4) verify no parent re-renders are remounting the observer.

### Shared UI Component Lock
The folder `src/components/ui/` is the project's **shared UI library** — it contains both official shadcn/ui primitives and project-authored common components built on top of shadcn. Both kinds are intended to be reused across the app and **must not be modified casually**.

- **Do not modify, refactor, restyle, or "improve"** any file under `src/components/ui/` (including subfolders), even if it seems like the most direct fix.
- If a component does not meet requirements, **wrap or compose it** in a new component **outside** `src/components/ui/` (e.g., under `src/components/` or a feature folder), and customize via `className`, `props`, or composition.
- Exceptions require **explicit user approval** in the same conversation. When in doubt, ask first and propose a wrapper-based alternative.
- Adding a new shared component to `src/components/ui/`, or upgrading a shadcn primitive via the official `shadcn` CLI, is allowed only when the user explicitly requests it.

### React Patterns and Conventions
- **Prefer `requestAnimationFrame` or `useLayoutEffect`** over `setTimeout(..., 0)` for focus or DOM measurement operations.
- **Prefer `useTranslation` from `react-i18next`** over project-wrapped utilities like `useTranslate`.
- Extract complex logic into hooks or utils; keep components lean.
- Use `PascalCase` for constants and component names.
- Avoid duplicating component structures in JSX; favor render props or reusable components.
