import { preprocessLaTeX } from '../chat';

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
