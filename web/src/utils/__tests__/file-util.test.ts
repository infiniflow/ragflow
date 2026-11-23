import { decodeBlobText } from '../file-util';

// jsdom's Blob class does not provide arrayBuffer().
const blobOf = (bytes: Uint8Array): Blob =>
  Object.assign(new Blob(), {
    arrayBuffer: () => Promise.resolve(bytes.buffer as ArrayBuffer),
  });

describe('decodeBlobText', () => {
  it('decodes UTF-8 with BOM and strips the leading U+FEFF', async () => {
    // 0xef 0xbb 0xbf is the UTF-8 BOM; TextDecoder strips it on its own,
    // so the result must start with 'H' (0x48), not U+FEFF.
    const payload = 'Hello, 世界';
    const bytes = new Uint8Array([
      0xef,
      0xbb,
      0xbf,
      ...new TextEncoder().encode(payload),
    ]);
    const got = await decodeBlobText(blobOf(bytes), 'windows-1252');
    expect(got).toBe(payload);
    expect(got.charCodeAt(0)).toBe('H'.charCodeAt(0));
  });

  it('decodes UTF-16LE with BOM (0xff 0xfe)', async () => {
    // "Hi" in UTF-16LE little-endian: H=0x48 0x00, i=0x69 0x00.
    const bytes = new Uint8Array([0xff, 0xfe, 0x48, 0x00, 0x69, 0x00]);
    const got = await decodeBlobText(blobOf(bytes));
    expect(got).toBe('Hi');
  });

  it('decodes UTF-16BE with BOM (0xfe 0xff)', async () => {
    // "Hi" in UTF-16BE big-endian: H=0x00 0x48, i=0x00 0x69.
    const bytes = new Uint8Array([0xfe, 0xff, 0x00, 0x48, 0x00, 0x69]);
    const got = await decodeBlobText(blobOf(bytes));
    expect(got).toBe('Hi');
  });

  it('decodes plain UTF-8 without a BOM', async () => {
    const bytes = new TextEncoder().encode('Hello, 世界');
    const got = await decodeBlobText(blobOf(bytes));
    expect(got).toBe('Hello, 世界');
  });

  it('uses the declared encoding for a file without a BOM', async () => {
    const bytes = new Uint8Array([0x63, 0x61, 0x66, 0xe9]);
    expect(await decodeBlobText(blobOf(bytes), 'windows-1252')).toBe('café');
  });

  it('falls back to GBK when strict UTF-8 throws (GB2312 Chinese text)', async () => {
    // "中文" in GBK: 中=0xd6 0xd0, 文=0xce 0xc4. These bytes are NOT valid
    // UTF-8, so the strict path throws and the GBK fallback decodes them.
    const bytes = new Uint8Array([0xd6, 0xd0, 0xce, 0xc4]);
    const got = await decodeBlobText(blobOf(bytes));
    expect(got).toBe('中文');
  });

  it('returns an empty string for an empty buffer', async () => {
    const got = await decodeBlobText(blobOf(new Uint8Array(0)));
    expect(got).toBe('');
  });

  it('decodes a 1-byte buffer without a false BOM match', async () => {
    // 0x41 ('A') alone is not a 2-byte or 3-byte BOM prefix, so the
    // function should still return the decoded byte rather than
    // misclassifying it. Strict UTF-8 succeeds for plain ASCII.
    const got = await decodeBlobText(blobOf(new Uint8Array([0x41])));
    expect(got).toBe('A');
  });

  it('falls through to GBK on a truncated UTF-8 BOM prefix', async () => {
    // 0xef 0xbb is the first two bytes of the 3-byte UTF-8 BOM
    // (0xef 0xbb 0xbf). Math.min(3, 2) yields a 2-byte slice, so
    // bytes[2] is undefined and the BOM check fails (undefined !== 0xbf).
    // Strict UTF-8 then throws on 0xbb as a stray continuation byte, and
    // the GBK fallback decodes the 2 bytes as a single GBK double-byte
    // character. Pinning this edge case catches a regression that
    // would otherwise treat a 2-byte 0xef 0xbb prefix as a complete BOM.
    const got = await decodeBlobText(blobOf(new Uint8Array([0xef, 0xbb])));
    // GBK double-byte yields exactly one character; we don't pin the
    // exact code point (GBK table mapping is implementation-defined)
    // but assert the function did not throw, did not return empty,
    // and did not leave a leading U+FEFF in the output.
    expect(got.length).toBe(1);
    expect(got.charCodeAt(0)).not.toBe(0xfeff);
  });
});
