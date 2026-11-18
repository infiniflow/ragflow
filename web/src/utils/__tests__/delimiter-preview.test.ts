import {
  parseDelimiterListForDisplay,
  parseDelimitersForDisplay,
} from '../delimiter-preview';

describe('parseDelimitersForDisplay', () => {
  it('returns empty for undefined or empty input', () => {
    expect(parseDelimitersForDisplay(undefined)).toEqual([]);
    expect(parseDelimitersForDisplay('')).toEqual([]);
  });

  it.each([
    ['!', ['!']],
    ['!?', ['!', '?']],
    [' ', [' ']],
    ['\n', ['\n']],
    ['\t', ['\t']],
    ['\r', ['\n']],
    ['\r\n', ['\n']],
    ['\n!?;。；！？', ['\n', '!', '?', ';', '。', '；', '！', '？']],
    ['`##`', ['##']],
    ['`###``##``#`', ['###', '##', '#']],
    ['\n`##`;', ['##', '\n', ';']],
    ['`a`a`a`', ['a']],
    ['`\n\n`', ['\n\n']],
    ['`\t\t`', ['\t\t']],
    ['é', ['é']],
    ['。', ['。']],
  ])('parses %j to match backend set/order', (field, expected) => {
    const got = parseDelimitersForDisplay(field).map((d) => d.raw);
    expect(got).toEqual(expected);
  });

  it('sorts longest-first', () => {
    const got = parseDelimitersForDisplay('`#``##``###`').map((d) => d.raw);
    expect(got).toEqual(['###', '##', '#']);
  });

  it('normalizes CRLF before parsing', () => {
    expect(parseDelimitersForDisplay('\r\n').map((d) => d.raw)).toEqual(['\n']);
    expect(parseDelimitersForDisplay('`\r\n`').map((d) => d.raw)).toEqual([
      '\n',
    ]);
  });

  it('applies whitespace glyphs only for display', () => {
    const [item] = parseDelimitersForDisplay('\n');
    expect(item.raw).toBe('\n');
    expect(item.display).toBe('↵');
  });
});

describe('parseDelimiterListForDisplay', () => {
  it('returns empty for undefined, empty or blank rows', () => {
    expect(parseDelimiterListForDisplay(undefined, { keepBare: true })).toEqual(
      [],
    );
    expect(parseDelimiterListForDisplay([], { keepBare: true })).toEqual([]);
    expect(
      parseDelimiterListForDisplay(['', undefined], { keepBare: true }),
    ).toEqual([]);
  });

  it('keeps every non-empty row as one delimiter when keepBare', () => {
    // A multi-character row stays one delimiter — unlike the single-string
    // field, where "##" would be two '#' delimiters.
    const got = parseDelimiterListForDisplay(['\n', '!', '##'], {
      keepBare: true,
    }).map((d) => d.raw);
    expect(got).toEqual(['##', '\n', '!']);
  });

  it('strips the backticks from a wrapped row', () => {
    const got = parseDelimiterListForDisplay(['`##`', '`END`'], {
      keepBare: true,
    }).map((d) => d.raw);
    expect(got).toEqual(['END', '##']);
  });

  it('ignores bare rows for the Python backend', () => {
    const got = parseDelimiterListForDisplay(['\n', '!', '`##`'], {
      keepBare: false,
    }).map((d) => d.raw);
    expect(got).toEqual(['##']);
  });

  it('dedupes a bare row and its backtick-wrapped twin', () => {
    const got = parseDelimiterListForDisplay(['#', '`#`', '###'], {
      keepBare: true,
    }).map((d) => d.raw);
    expect(got).toEqual(['###', '#']);
  });

  it('applies whitespace glyphs to a list row', () => {
    const [item] = parseDelimiterListForDisplay(['\n'], { keepBare: true });
    expect(item.raw).toBe('\n');
    expect(item.display).toBe('↵');
  });
});
