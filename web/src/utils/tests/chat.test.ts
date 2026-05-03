import { preprocessLaTeX } from '../chat';

test('handles double-escaped inline LaTeX', () => {
  const result = preprocessLaTeX('\\\\(\\\\Delta\\\\)');
  expect(result).toBe('$\\Delta$');
});

test('handles double-escaped block LaTeX', () => {
  const result = preprocessLaTeX('\\\\[x = 1\\\\]');
  expect(result).toBe('$$x = 1$$');
});

test('decodes HTML entities in equations', () => {
  const result = preprocessLaTeX('x &lt; 0 and x &gt; 0');
  expect(result).toBe('x < 0 and x > 0');
});

test('decodes &amp; HTML entity', () => {
  const result = preprocessLaTeX('a &amp; b');
  expect(result).toBe('a & b');
});