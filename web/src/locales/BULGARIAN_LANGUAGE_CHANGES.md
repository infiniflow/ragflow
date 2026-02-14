# Add Bulgarian Language Support

**Repository:** https://github.com/trifonnt/ragflow

## Overview

Added Bulgarian (`bg` / `Български`) as the 13th supported UI language in RAGFlow.

## Changed Files

### `web/src/constants/common.ts`

Registered Bulgarian in all 5 language data structures:

- **`LanguageList`** — Added `'Bulgarian'` to the array of available languages
- **`LanguageMap`** — Added `Bulgarian: 'Български'` mapping English name to native name
- **`LanguageAbbreviation`** enum — Added `Bg = 'bg'` using ISO 639-1 code
- **`LanguageAbbreviationMap`** — Added `[LanguageAbbreviation.Bg]: 'Български'` for the language selector display
- **`LanguageTranslationMap`** — Added `Bulgarian: 'bg'` for language code resolution

### `web/src/locales/config.ts`

Added a lazy-loading dynamic import entry for the Bulgarian locale file:

```typescript
[LanguageAbbreviation.Bg]: () => import('./bg'),
```

This ensures the Bulgarian translation bundle is only loaded on demand when a user selects it.

### `web/src/locales/bg.ts` (new file)

Created the full Bulgarian translation file containing all 26 sections and 2001 translation keys, matching the English source (`en.ts`) exactly:

`common`, `login`, `header`, `memories`, `memory`, `knowledgeList`, `knowledgeDetails`, `knowledgeConfiguration`, `chunk`, `chat`, `setting`, `message`, `fileManager`, `flow`, `llmTools`, `modal`, `mcp`, `search`, `language`, `pagination`, `dataflowParser`, `datasetOverview`, `deleteModal`, `empty`, `admin`, `explore`

All interpolation placeholders (`{{variable}}`), HTML tags, and technical terms (model names, URLs, API references) are preserved as-is.

### `deepdoc/parser/mineru_parser.py`

Added Bulgarian to the `LANGUAGE_TO_MINERU_MAP` dictionary for OCR/PDF parser language support:

```python
'Bulgarian': 'cyrillic',
```

Bulgarian uses the Cyrillic script, so the `'cyrillic'` MinerU language code is used.

## How It Works

- The language selector in the header automatically picks up the new entry from `LanguageList` and `LanguageMap`
- When a user selects "Български", `changeLanguageAsync('bg')` lazy-loads `bg.ts` and switches the UI
- The user's preference is saved to the database and localStorage for persistence across sessions
- `supportedLngs` in i18next is derived from `Object.values(LanguageAbbreviation)`, so adding `Bg` to the enum automatically registers it
