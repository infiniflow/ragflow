import {
  preprocessLaTeX,
  replaceRetrievingToSection,
  replaceThinkToSection,
} from '../chat';

describe('preprocessLaTeX', () => {
  it('converts block \\[ \\] to $$ $$', () => {
    expect(preprocessLaTeX('\\[ x + y \\]')).toBe('$$x + y$$');
  });

  it('converts inline \\( \\) to $ $', () => {
    expect(preprocessLaTeX('\\( a \\)')).toBe('$a$');
  });

  it('does not cut block math at \\right] (Closes #13134)', () => {
    const content =
      '\\[ C_{seq}(y|x) = \\frac{1}{|y|} \\sum_{t=1}^{|y|} \\right] \\]';
    const result = preprocessLaTeX(content);
    expect(result).toContain('\\right]');
    expect(result).toContain('\\frac{1}{|y|}');
    expect(result).toBe(
      '$$ C_{seq}(y|x) = \\frac{1}{|y|} \\sum_{t=1}^{|y|} \\right] $$',
    );
  });

  it('does not cut inline math at \\big) or nested parens', () => {
    const content = '\\( f(x) + \\big) \\)';
    const result = preprocessLaTeX(content);
    expect(result).toContain('\\big)');
    expect(result).toBe('$ f(x) + \\big) $');
  });

  it('handles multiple block equations', () => {
    const content = 'First \\[ a \\] then \\[ b \\right] c \\]';
    const result = preprocessLaTeX(content);
    expect(result).toBe('First $$a$$ then $$ b \\right] c $$');
  });
});

describe('reasoning block markdown boundaries', () => {
  it('keeps markdown parseable after a think block', () => {
    const result = replaceThinkToSection('<think>reasoning</think>### Heading');
    expect(result).toContain('</details>\n\n### Heading');
  });

  it('keeps markdown parseable after a retrieving block', () => {
    const result = replaceRetrievingToSection(
      '<retrieving>sources</retrieving>| A | B |\n| --- | --- |\n| 1 | 2 |',
    );
    expect(result).toContain(
      '</details>\n\n| A | B |\n| --- | --- |\n| 1 | 2 |',
    );
  });

  it('keeps markdown parseable for a list after a think block', () => {
    const result = replaceThinkToSection('<think>reasoning</think>1. First');
    expect(result).toContain('</details>\n\n1. First');
  });
});
