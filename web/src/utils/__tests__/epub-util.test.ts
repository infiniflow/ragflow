/** @jest-environment node */

import JSZip from 'jszip';
import { normalizeEpubEncoding } from '../epub-util';

jest.mock('@/services/file-manager-service', () => ({
  __esModule: true,
  default: {},
}));

const ChapterPath = 'OEBPS/chapter.xhtml';
const BinaryPath = 'OEBPS/image.png';
const BinaryData = new Uint8Array([0, 255, 128, 42]);

async function createArchive(chapter: Uint8Array): Promise<ArrayBuffer> {
  const zip = new JSZip();
  zip.file('mimetype', 'application/epub+zip');
  zip.file(ChapterPath, chapter);
  zip.file(BinaryPath, BinaryData);
  return zip.generateAsync({ type: 'arraybuffer' });
}

function gbkChapter(prolog: string): Uint8Array {
  return new Uint8Array([
    ...new TextEncoder().encode(`${prolog}<html><body>`),
    0xd6,
    0xd0,
    0xce,
    0xc4,
    ...new TextEncoder().encode('</body></html>'),
  ]);
}

function utf16Chapter(text: string, littleEndian: boolean): Uint8Array {
  const bytes = new Uint8Array(2 + text.length * 2);
  bytes.set(littleEndian ? [0xff, 0xfe] : [0xfe, 0xff]);
  const view = new DataView(bytes.buffer);
  for (let i = 0; i < text.length; i++) {
    view.setUint16(2 + i * 2, text.charCodeAt(i), littleEndian);
  }
  return bytes;
}

describe('normalizeEpubEncoding', () => {
  it.each([
    '',
    '<?xml version="1.0"?>',
    '<?xml version="1.0" encoding="UTF-8"?>',
  ])(
    'converts GBK bytes even when the declaration does not change: %p',
    async (prolog) => {
      const input = await createArchive(gbkChapter(prolog));
      const output = await normalizeEpubEncoding(input);
      const zip = await JSZip.loadAsync(output);
      const bytes = await zip.file(ChapterPath)!.async('uint8array');

      expect(new TextDecoder('utf-8', { fatal: true }).decode(bytes)).toBe(
        `${prolog}<html><body>中文</body></html>`,
      );
      expect(await zip.file(ChapterPath)!.async('string')).toBe(
        `${prolog}<html><body>中文</body></html>`,
      );
      expect(await zip.file(BinaryPath)!.async('uint8array')).toEqual(
        BinaryData,
      );
      expect(await zip.file('mimetype')!.async('string')).toBe(
        'application/epub+zip',
      );
    },
  );

  it.each([true, false])(
    'converts BOM-marked UTF-16 without an encoding declaration (LE=%p)',
    async (littleEndian) => {
      const text = '<?xml version="1.0"?><html><body>中文</body></html>';
      const input = await createArchive(utf16Chapter(text, littleEndian));
      const zip = await JSZip.loadAsync(await normalizeEpubEncoding(input));
      const bytes = await zip.file(ChapterPath)!.async('uint8array');

      expect(Array.from(bytes)).toEqual(
        Array.from(new TextEncoder().encode(text)),
      );
      expect(await zip.file(ChapterPath)!.async('string')).toBe(text);
    },
  );

  it('still rewrites an explicit legacy encoding declaration', async () => {
    const input = await createArchive(
      gbkChapter('<?xml version="1.0" encoding="GB2312"?>'),
    );
    const zip = await JSZip.loadAsync(await normalizeEpubEncoding(input));

    expect(await zip.file(ChapterPath)!.async('string')).toBe(
      '<?xml version="1.0" encoding="UTF-8"?><html><body>中文</body></html>',
    );
  });

  it('returns the original archive when text entries are already UTF-8', async () => {
    const text = '<?xml version="1.0" encoding="UTF-8"?><html>中文</html>';
    const input = await createArchive(new TextEncoder().encode(text));

    expect(await normalizeEpubEncoding(input)).toBe(input);
  });

  it('leaves archives with only binary entries unchanged', async () => {
    const zip = new JSZip();
    zip.file(BinaryPath, BinaryData);
    const input = await zip.generateAsync({ type: 'arraybuffer' });

    expect(await normalizeEpubEncoding(input)).toBe(input);
  });
});
