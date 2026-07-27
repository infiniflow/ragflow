/**
 * Parses a "Delimiter for text" field value into a list of delimiters for
 * display in the UI. The result is meant to be informative only — the
 * actual chunking behavior is determined by the backend.
 *
 * The parsing rule mirrors `rag/nlp/__init__.py::get_delimiters` in
 * spirit (backticks group characters into a multi-character delimiter;
 * anything outside backticks is itself a delimiter) but does **not**
 * exactly match any of the six divergent backend implementations, since
 * those implementations disagree on dedupe, sort order, the use of
 * `re.I`, and CRLF normalization. The helper is a *preview*, not a
 * contract. See #17383 for the backend consolidation.
 */
export interface ParsedDelimiter {
  /** The original characters the user entered. */
  raw: string;
  /**
   * The same content with whitespace characters replaced by visible
   * Unicode glyphs so that whitespace-only delimiters (which are
   * invisible in a single-line input) still appear in the preview.
   */
  display: string;
}

/**
 * Map of whitespace characters to visible glyphs. Only used for
 * display; the value sent to the backend is the raw string.
 */
const WHITESPACE_GLYPHS: Record<string, string> = {
  '\n': '↵', // DOWNWARDS ARROW WITH CORNER LEFTWARDS
  '\t': '⇥', // RIGHTWARDS ARROW TO BAR
  '\r': '␍', // CARRIAGE RETURN SYMBOL
  ' ': '␣', // OPEN BOX
  '\f': '␌', // FORM FEED SYMBOL
  '\v': '␋', // VERTICAL TAB SYMBOL
};

/**
 * Render a raw delimiter string for display by substituting visible
 * Unicode glyphs for each whitespace character. Non-whitespace
 * characters pass through unchanged.
 */
function toDisplay(raw: string): string {
  let out = '';
  for (const ch of raw) {
    out += ch in WHITESPACE_GLYPHS ? WHITESPACE_GLYPHS[ch] : ch;
  }
  return out;
}

/**
 * Parse the delimiter field into a deduplicated, display-ready list.
 *
 * Differences from the backend `get_delimiters`:
 *  - Deduplicates by raw value so distinct delimiters that happen to
 *    share a display glyph (e.g. a literal `\n` and a user-typed `↵`)
 *    are not silently merged. The backend does not always dedupe (see
 *    #17383).
 *  - Preserves left-to-right input order rather than sorting
 *    longest-first; visual order is more intuitive for users.
 *  - Does not regex-escape the values (irrelevant for display).
 *
 * Returns an empty array if the value is undefined or empty.
 */
export function parseDelimitersForDisplay(
  value: string | undefined,
): ParsedDelimiter[] {
  if (!value) return [];

  const result: ParsedDelimiter[] = [];
  const seen = new Set<string>();

  const push = (raw: string) => {
    if (seen.has(raw)) return;
    seen.add(raw);
    result.push({ raw, display: toDisplay(raw) });
  };

  let cursor = 0;
  for (const match of value.matchAll(/`([^`]+)`/g)) {
    const start = match.index!;
    const end = start + match[0].length;
    // bare characters before this backtick-wrapped token
    for (const ch of value.slice(cursor, start)) push(ch);
    // the backtick-wrapped token
    push(match[1]);
    cursor = end;
  }
  // bare characters after the last token (or the whole string if no
  // backticks were present)
  for (const ch of value.slice(cursor)) push(ch);

  return result;
}
