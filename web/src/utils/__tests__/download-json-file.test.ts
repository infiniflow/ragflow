import { downloadJsonFile } from '../file-util';

jest.mock('@/services/file-manager-service', () => ({
  __esModule: true,
  default: {},
}));

describe('downloadJsonFile', () => {
  let exportedBlob: Blob;
  const originalCreateObjectURL = URL.createObjectURL;
  const originalRevokeObjectURL = URL.revokeObjectURL;

  beforeEach(() => {
    URL.createObjectURL = jest.fn((blob: Blob) => {
      exportedBlob = blob;
      return 'blob:json-export';
    });
    URL.revokeObjectURL = jest.fn();
    jest
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
    URL.createObjectURL = originalCreateObjectURL;
    URL.revokeObjectURL = originalRevokeObjectURL;
  });

  async function exportText(data: Record<string, unknown>) {
    await downloadJsonFile(data, 'output.json');
    return new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as string);
      reader.onerror = () => reject(reader.error);
      reader.readAsText(exportedBlob);
    });
  }

  it.each([null, 'literal', { z: 2, a: 1 }])(
    'preserves an own __proto__ key with value %p',
    async (value) => {
      const data = JSON.parse(`{"__proto__":${JSON.stringify(value)}}`);
      const text = await exportText(data);

      expect(JSON.parse(text)).toEqual(data);
      expect(Object.hasOwn(JSON.parse(text), '__proto__')).toBe(true);
      expect(Object.getPrototypeOf(data)).toBe(Object.prototype);
    },
  );

  it('preserves special keys in nested objects and array elements', async () => {
    const data = JSON.parse(
      '{"z":0,"nested":{"__proto__":{"z":2,"a":1},"constructor":"value"},"items":[{"__proto__":null},{"__proto__":"literal"}]}',
    );
    const before = JSON.stringify(data);
    const text = await exportText(data);

    expect(JSON.parse(text)).toEqual(data);
    expect(JSON.stringify(data)).toBe(before);
    expect(text).toContain(
      '"__proto__": {\n      "a": 1,\n      "z": 2\n    }',
    );
  });

  it('keeps sorted pretty printing, array order and Date serialization', async () => {
    const text = await exportText({
      z: null,
      a: [{ z: 2, a: 1 }, 'second'],
      date: new Date('2026-01-01T00:00:00.000Z'),
    });

    expect(text).toBe(
      JSON.stringify(
        {
          a: [{ a: 1, z: 2 }, 'second'],
          date: '2026-01-01T00:00:00.000Z',
          z: null,
        },
        null,
        2,
      ),
    );
    expect(exportedBlob.type).toBe('application/json');
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:json-export');
  });
});
