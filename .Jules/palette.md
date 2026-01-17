## 2026-01-17 - Uncovering Hidden Accessibility Features
**Learning:** I discovered that developers sometimes comment out accessibility features (like `aria-label`) because they lack localization support or are unsure about the implementation.
**Action:** Always check commented-out code in UI components for accessibility features that can be easily re-enabled, even if it requires a hardcoded fallback or a small workaround.

## 2026-01-17 - Focus Styles in Tailwind
**Learning:** Custom components that override background colors (e.g., `bg-bg-card`) can inadvertently hide default focus indicators if they rely on background color changes.
**Action:** When overriding background colors on interactive elements, always explicitly define `focus-visible` styles (like `ring-2`) to ensuring keyboard navigability is maintained.
