import { parseDelimitersForDisplay } from '../delimiter-preview';

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
