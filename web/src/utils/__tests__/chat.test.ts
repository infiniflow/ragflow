import {
  mergeAnswerChunk,
  preprocessLaTeX,
  replaceThinkToSection,
} from '../chat';

describe('mergeAnswerChunk', () => {
  it.each([
    ['First fact. Second fact.', 'First fact.[ID:0] Second fact.[ID:1]'],
    ['A fact.[ID:0-1]', 'A fact.[ID:0][ID:1]'],
  ])('replaces %s with the final cited answer', (previous, answer) => {
    expect(mergeAnswerChunk(previous, { answer, final: true })).toBe(answer);
  });

  it('keeps streamed thinking when replacing the visible answer', () => {
    const answer = [
      { start_to_think: true },
      { answer: 'Checking sources.' },
      { end_to_think: true },
      { answer: 'First fact. ' },
      { answer: 'Second fact.' },
      { answer: 'First fact.[ID:0] Second fact.[ID:1]', final: true },
    ].reduce(mergeAnswerChunk, '');

    expect(answer).toBe(
      '<think>Checking sources.</think>First fact.[ID:0] Second fact.[ID:1]',
    );
    expect(mergeAnswerChunk(answer, { answer, final: true })).toBe(answer);
  });

  it.each(['', undefined])(
    'keeps the answer for an empty final chunk (%s)',
    (answer) => {
      const previous = '<think>Checking sources.</think>A fact.[ID:0]';
      expect(mergeAnswerChunk(previous, { answer, final: true })).toBe(
        previous,
      );
    },
  );

  it('accepts a single-shot final answer without duplicating a repeated one', () => {
    const chunk = { answer: 'A fact.[ID:0]', final: true };
    const answer = mergeAnswerChunk('', chunk);
    expect(answer).toBe(chunk.answer);
    expect(mergeAnswerChunk(answer, chunk)).toBe(answer);
  });
});

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

  it('handles double-escaped inline LaTeX', () => {
    expect(preprocessLaTeX('\\\\(\\\\Delta = b^2\\\\)')).toBe(
      '$\\Delta = b^2$',
    );
  });

  it('handles double-escaped block LaTeX', () => {
    expect(preprocessLaTeX('\\\\[E = mc^2\\\\]')).toBe('$$E = mc^2$$');
  });

  it('decodes HTML entities', () => {
    expect(preprocessLaTeX('a &lt; b &amp; c &gt; d')).toBe('a < b & c > d');
  });

  it('handles mixed double-escaped delimiters with HTML entities', () => {
    expect(preprocessLaTeX('\\\\(x &lt; y\\\\)')).toBe('$x < y$');
  });

  it('passes through already correct single-escaped delimiters unchanged', () => {
    expect(preprocessLaTeX('\\(x = 1\\)')).toBe('$x = 1$');
  });
});

describe('replaceThinkToSection', () => {
  it('drops an empty think section instead of rendering a bare strip', () => {
    expect(replaceThinkToSection('<think></think>Here is the answer.')).toBe(
      'Here is the answer.',
    );
  });

  it('drops a whitespace-only think section', () => {
    expect(replaceThinkToSection('<think>  \n </think>answer')).toBe('answer');
  });

  it('keeps a non-empty think section as a details block', () => {
    expect(replaceThinkToSection('<think>some reasoning</think>answer')).toBe(
      '<details class="think"><summary>Thinking...</summary>some reasoning</details>answer',
    );
  });

  it('uses the provided summary for non-empty sections', () => {
    expect(
      replaceThinkToSection('<think>reasoning</think>', 'Deep thought'),
    ).toBe(
      '<details class="think"><summary>Deep thought</summary>reasoning</details>',
    );
  });

  it('leaves text without think markers unchanged', () => {
    expect(replaceThinkToSection('plain answer')).toBe('plain answer');
  });
});
