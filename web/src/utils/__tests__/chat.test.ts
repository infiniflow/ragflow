import {
  countAgenticLogLines,
  isAgenticLogLine,
  preprocessLaTeX,
  promoteCaretExponentsToLaTeX,
  replaceAgenticLogsToSection,
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

  it('gives an Agentic RAG think body the log panel instead of the reasoning label', () => {
    const result = replaceThinkToSection(
      '<think>[Agentic RAG] Starting research...\n[Keywords] cable</think>Answer',
      'Thought',
      'Log · {{num}}',
    );

    expect(result).toContain('<details class="agentic-log">');
    expect(result).toContain('<summary>Log · 2</summary>');
    expect(result).not.toContain('class="think"');
    expect(result.endsWith('Answer')).toBe(true);
  });

  it('keeps a reasoning think body on the generic summary when a log summary is passed', () => {
    const result = replaceThinkToSection(
      '<think>Step one.\nStep two.</think>Answer',
      'Thought',
      'Log · {{num}}',
    );

    expect(result).toBe(
      '<details class="think"><summary>Thought</summary>Step one.\nStep two.</details>Answer',
    );
  });

  it('handles the Python wire shape where each log line is <br>-prefixed', () => {
    const result = replaceThinkToSection(
      '<think><br>[Hybrid search] Searching the knowledge base for "x"\n<br>[Keywords] cable\n</think>Answer',
      'Thought',
      'Log · {{num}}',
    );

    expect(result).toContain('<details class="agentic-log">');
    expect(result).toContain('<summary>Log · 2</summary>');
    expect(result).toContain('<div>[Hybrid search] Searching the knowledge base for "x"</div>');
    expect(result).toContain('<div>[Keywords] cable</div>');
    expect(result.endsWith('Answer')).toBe(true);
  });

  it('handles the Go wire shape where each log line ends with <br> and no newline', () => {
    const result = replaceThinkToSection(
      '<think>[Agentic RAG] Starting research — mode=hybrid<br>[Keywords] entity x1: copper<br></think>Answer',
      'Thought',
      'Log · {{num}}',
    );

    expect(result).toContain('<summary>Log · 2</summary>');
    expect(result).toContain('<div>[Agentic RAG] Starting research — mode=hybrid</div>');
    expect(result).toContain('<div>[Keywords] entity x1: copper</div>');
    expect(result).not.toContain('<br>');
    expect(result.endsWith('Answer')).toBe(true);
  });

  it('collapses a still-open think block so streaming never leaks logs', () => {
    const result = replaceThinkToSection(
      '<think>[Direct search] Looking up the knowledge base for: "x"',
      'Thought',
      'Log · {{num}}',
    );

    expect(result).toContain('<details class="agentic-log">');
    expect(result).toContain('<summary>Log · 1</summary>');
  });
});

describe('agentic RAG log extraction', () => {
  it('recognises every forwarded stage prefix', () => {
    expect(isAgenticLogLine('[Agentic RAG] Starting research...')).toBe(true);
    expect(isAgenticLogLine('[Formalize] query rewritten')).toBe(true);
    expect(isAgenticLogLine('[Keywords] "cable tray"')).toBe(true);
    expect(isAgenticLogLine('[Direct search] question')).toBe(true);
    expect(isAgenticLogLine('[Hybrid search] question')).toBe(true);
    expect(isAgenticLogLine('- [Keywords] list item form')).toBe(true);
    expect(isAgenticLogLine('Regular answer text')).toBe(false);
  });

  it('counts only the log lines', () => {
    expect(
      countAgenticLogLines('[Keywords] a\nAnswer text\n[Hybrid search] b'),
    ).toBe(2);
  });

  it('collapses bare log lines into one collapsed panel and keeps the answer clean', () => {
    const result = replaceAgenticLogsToSection(
      '[Agentic RAG] Starting research...\n[Keywords] 1.5mm^2\nHere is the answer.',
      'Log · {{num}}',
    );

    expect(result).toContain('<details class="agentic-log">');
    expect(result).toContain('<summary>Log · 2</summary>');
    expect(result).toContain('<div>[Keywords] 1.5mm^2</div>');
    expect(result).toContain('Here is the answer.');
    // The raw log lines must not survive outside the collapsed panel.
    expect(result).not.toContain('\n[Agentic RAG]');
  });

  it('leaves fenced code blocks untouched', () => {
    const content = '```\n[Keywords] is not a log here\n```\nDone.';
    expect(replaceAgenticLogsToSection(content, 'Log · {{num}}')).toBe(content);
  });

  it('extracts <br>-separated logs that were never wrapped in a think block', () => {
    const result = replaceAgenticLogsToSection(
      '[Hybrid search] Searching for "x"<br>[Keywords] copper<br>The conductor is 1.5mm^2.',
      'Log · {{num}}',
    );

    expect(result).toContain('<summary>Log · 2</summary>');
    expect(result).toContain('<div>[Keywords] copper</div>');
    expect(result).toContain('The conductor is 1.5mm^2.');
  });

  it('keeps the answer text of a line that mixes a log with content', () => {
    const result = replaceAgenticLogsToSection(
      '[Keywords] copper<br>Answer paragraph.',
      'Log · {{num}}',
    );

    expect(result).toContain('<summary>Log · 1</summary>');
    expect(result).toContain('Answer paragraph.');
  });

  it('returns the input unchanged when there is nothing to extract', () => {
    expect(replaceAgenticLogsToSection('Just the answer.', 'Log')).toBe(
      'Just the answer.',
    );
  });
});

describe('promoteCaretExponentsToLaTeX', () => {
  it('promotes cable-unit exponents typed as plain text', () => {
    expect(promoteCaretExponentsToLaTeX('1.5mm^2 conductor')).toBe(
      '1.5mm$^{2}$ conductor',
    );
    expect(promoteCaretExponentsToLaTeX('10^-6 m^3')).toBe(
      '10$^{-6}$ m$^{3}$',
    );
    expect(promoteCaretExponentsToLaTeX('m^{3}/s')).toBe('m$^{3}$/s');
  });

  it('does not touch code spans, fenced code or existing math', () => {
    expect(promoteCaretExponentsToLaTeX('use `x^2` here')).toBe(
      'use `x^2` here',
    );
    expect(promoteCaretExponentsToLaTeX('$a^2 + b^2$')).toBe('$a^2 + b^2$');
    expect(promoteCaretExponentsToLaTeX('```\nx^2\n```')).toBe('```\nx^2\n```');
  });

  it('leaves text without a caret untouched', () => {
    expect(promoteCaretExponentsToLaTeX('plain text')).toBe('plain text');
  });
});
