import { defaultSchema } from 'hast-util-sanitize';
import rehypeSanitize from 'rehype-sanitize';
import type { PluggableList } from 'unified';

/**
 * Allow-list for assistant-rendered markdown.
 *
 * The renderers run `rehype-raw`, so raw HTML inside a message becomes real
 * nodes. Sanitizing the markdown *string* before that cannot hold:
 * `preprocessLaTeX()` decodes `&lt;`/`&gt;`/`&amp;` back into markup afterwards,
 * so an entity-encoded payload is inert while the allow-list inspects it and
 * live again by the time rehype-raw parses it. Sanitizing the tree closes that
 * gap, and it leaves text nodes alone -- which a string-level pass cannot do,
 * because escaping `<` breaks `$a < b$` for KaTeX and drops `Array<number>`
 * inside code fences.
 */
export const MarkdownSanitizeSchema = {
  ...defaultSchema,
  tagNames: [
    ...(defaultSchema.tagNames ?? []),
    // wrappers injected by replaceThinkToSection / replaceRetrievingToSection
    'think',
    'retrieving',
    'section',
    'details',
    'summary',
  ],
  attributes: {
    ...defaultSchema.attributes,
    '*': [...(defaultSchema.attributes?.['*'] ?? []), 'className'],
  },
  protocols: {
    ...defaultSchema.protocols,
    // Assistant answers legitimately embed inline images as `data:` URLs
    // (base64 charts, screenshots). `img` is the only src-bearing tag left in
    // the allow-list -- script/iframe/object/embed are not in `tagNames` -- and
    // a `data:` payload loaded through <img> cannot execute script, so this
    // re-enables inline images without re-opening a script sink.
    src: [...(defaultSchema.protocols?.src ?? []), 'data'],
  },
};

/**
 * Apply the allow-list to the parsed tree. Keep this after `rehype-raw` and
 * before `rehype-katex`: KaTeX emits markup the allow-list does not cover.
 */
export const RehypeSanitizeAssistantMarkdown: PluggableList[number] = [
  rehypeSanitize,
  MarkdownSanitizeSchema,
];
