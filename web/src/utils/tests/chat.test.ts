import { preprocessLaTeX, sanitizeMarkdown } from '../chat';

test('handles double-escaped inline LaTeX', () => {
  const result = preprocessLaTeX('\\\\(\\\\Delta = b^2\\\\)');
  expect(result).toBe('$\\Delta = b^2$');
});

test('handles double-escaped block LaTeX', () => {
  const result = preprocessLaTeX('\\\\[E = mc^2\\\\]');
  expect(result).toBe('$$E = mc^2$$');
});

test('decodes HTML entities', () => {
  const result = preprocessLaTeX('a &lt; b &amp; c &gt; d');
  expect(result).toBe('a < b & c > d');
});

test('handles mixed double-escaped delimiters with HTML entities', () => {
  const result = preprocessLaTeX('\\\\(x &lt; y\\\\)');
  expect(result).toBe('$x < y$');
});

test('passes through already correct single-escaped delimiters unchanged', () => {
  const result = preprocessLaTeX('\\(x = 1\\)');
  expect(result).toBe('$x = 1$');
});

// preprocessLaTeX() intentionally decodes &lt;/&gt;/&amp;, so sanitizing before
// it runs is not effective: the decode turns inert text back into live markup
// that rehypeRaw then parses. sanitizeMarkdown() must therefore run last.
test('sanitizeMarkdown drops markup that preprocessLaTeX decoded', () => {
  const decoded = preprocessLaTeX(
    'hi &lt;svg&gt;&lt;script&gt;alert(1)&lt;/script&gt;&lt;/svg&gt;',
  );
  expect(decoded).toContain('<script>');

  const sanitized = sanitizeMarkdown(decoded);
  expect(sanitized).not.toContain('<script>');
  expect(sanitized).toContain('hi');
});

test('sanitizeMarkdown drops event-handler attributes', () => {
  const sanitized = sanitizeMarkdown(
    preprocessLaTeX('&lt;img src=x onerror="alert(1)"&gt;'),
  );
  expect(sanitized).not.toContain('onerror');
});

test('sanitizeMarkdown keeps the tags the chat renderers rely on', () => {
  const sanitized = sanitizeMarkdown(
    '<details class="think"><summary>Thinking...</summary>why</details>',
  );
  expect(sanitized).toContain('<details class="think">');
  expect(sanitized).toContain('<summary>');
});
