import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';

/**
 * GFM + line breaks only (no TeX). For surfaces that do not wire rehype-katex
 * (e.g. uploaded document preview).
 */
export const MarkdownRemarkPluginsLite = [remarkGfm, remarkBreaks];

/**
 * Shared Markdown pipeline for assistant-style content:
 * - remark-gfm: GFM tables, task lists, strikethrough, autolinks, etc.
 * - remark-math: TeX ($...$ / $$...$$); pair with rehype-katex on render.
 * - remark-breaks: treat single newlines as hard breaks (common in LLM chat).
 */
export const MarkdownRemarkPlugins = [remarkGfm, remarkMath, remarkBreaks];
