/** @jest-environment node */

import { decodeHtmlPreview } from './html-encoding';

function decode(parts: BlobPart[], type = 'text/html') {
  return decodeHtmlPreview(new Blob(parts, { type }));
}

const CommentedCharset = '<!-- <meta charset="windows-1252"> -->';
const HtmlText = '<p>café</p>';

describe('decodeHtmlPreview', () => {
  it('skips comments during detection and preserves them in the result', async () => {
    const html = CommentedCharset + '<meta charset="utf-8">' + HtmlText;
    await expect(decode([html])).resolves.toBe(html);
  });

  it('ignores a comment extending past the scan window', async () => {
    const html =
      '<!-- <meta charset="windows-1252">' +
      ' '.repeat(1024) +
      ' -->' +
      HtmlText;
    await expect(decode([html])).resolves.toBe(html);
  });

  it('ignores markup and charset lookalikes inside attributes', async () => {
    const html =
      `<div data-html='<meta charset="windows-1252">'></div>` +
      '<meta name="description" content="café; charset=windows-1252">' +
      '<meta data-note="charset=windows-1252"><meta charset=utf-8>' +
      HtmlText;
    await expect(decode([html])).resolves.toBe(html);
  });

  it('uses the first effective charset and recognizes a legacy content-type declaration', async () => {
    const head =
      '<meta charset="not-an-encoding">' +
      '<META content="text/html; charset=Big5" http-equiv="Content-Type">' +
      '<meta charset="windows-1252"><p>';
    await expect(
      decode([head, new Uint8Array([0xa4, 0xa4, 0xa4, 0xe5]), '</p>']),
    ).resolves.toBe(head + '中文</p>');
  });

  it('ignores a meta tag whose closing bracket is past the scan window', async () => {
    const html = ' '.repeat(1000) + '<meta charset="windows-1252">' + HtmlText;
    await expect(decode([html])).resolves.toBe(html);
  });

  it('reads a real non-UTF-8 charset outside a comment', async () => {
    const head =
      '<!-- <meta charset="utf-8"> --><meta charset=windows-1252><p>caf';
    await expect(decode([head, new Uint8Array([0xe9]), '</p>'])).resolves.toBe(
      head + 'é</p>',
    );
  });

  it('gives the HTTP charset priority over a meta declaration', async () => {
    const html = '<meta charset="windows-1252">' + HtmlText;
    await expect(decode([html], 'text/html; charset=utf-8')).resolves.toBe(
      html,
    );
  });

  it('gives a BOM priority over both HTTP and meta declarations', async () => {
    const html = '<meta charset="windows-1252">' + HtmlText;
    await expect(
      decode(
        [new Uint8Array([0xef, 0xbb, 0xbf]), html],
        'text/html; charset=windows-1252',
      ),
    ).resolves.toBe(html);
  });

  it('falls back to GBK when there is no effective declaration', async () => {
    await expect(
      decode([
        '<!-- <meta charset="windows-1252"> --><p>',
        new Uint8Array([0xd6, 0xd0, 0xce, 0xc4]),
        '</p>',
      ]),
    ).resolves.toContain('<p>中文</p>');
  });
});
