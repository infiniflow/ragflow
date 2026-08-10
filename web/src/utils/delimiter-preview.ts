/**
 * Parses a "Delimiter for text" field value into a list of delimiters for
 * display in the UI. Mirrors the canonical backend parser in
 * `rag/nlp/delim.py` (`parse_delimiter_field`): CRLF normalization, bare
 * and backtick-wrapped tokens, insertion-ordered dedupe, and longest-first
 * stable sort. Whitespace glyph substitution is display-only.
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
 * Parse the delimiter field into a deduplicated, longest-first list that
 * matches backend `parse_delimiter_field` semantics. Display glyphs are
 * applied after parsing.
 *
 * Returns an empty array if the value is undefined or empty.
 */
export function parseDelimitersForDisplay(
  value: string | undefined,
): ParsedDelimiter[] {
  if (!value) return [];

  // Match backend: \r\n → \n, then standalone \r → \n.
  const normalizedValue = value.replaceAll('\r\n', '\n').replaceAll('\r', '\n');

  const result: ParsedDelimiter[] = [];
  const seen = new Set<string>();

  const push = (raw: string) => {
    if (!raw || seen.has(raw)) return;
    seen.add(raw);
    result.push({ raw, display: toDisplay(raw) });
  };

  let cursor = 0;
  for (const match of normalizedValue.matchAll(/`([^`]+)`/g)) {
    const start = match.index!;
    const end = start + match[0].length;
    for (const ch of normalizedValue.slice(cursor, start)) push(ch);
    push(match[1]);
    cursor = end;
  }
  for (const ch of normalizedValue.slice(cursor)) push(ch);

  // Stable sort longest-first (matches backend parse_delimiter_field).
  return result.sort((a, b) => b.raw.length - a.raw.length);
}
