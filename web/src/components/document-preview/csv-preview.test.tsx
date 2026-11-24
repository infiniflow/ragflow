import { render, screen } from '@testing-library/react';
import { Blob as NodeBlob } from 'node:buffer';
import CSVFileViewer from './csv-preview';

const mockRequest = jest.fn();
jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: (...args: unknown[]) => mockRequest(...args),
}));

const OriginalResizeObserver = globalThis.ResizeObserver;
const OriginalBlob = globalThis.Blob;
beforeAll(() => {
  // jsdom's Blob lacks arrayBuffer(), which the preview uses to decode the file.
  Object.defineProperty(globalThis, 'Blob', { value: NodeBlob });
  globalThis.ResizeObserver = jest.fn().mockImplementation(() => ({
    observe: jest.fn(),
    disconnect: jest.fn(),
  }));
});
afterAll(() => {
  Object.defineProperty(globalThis, 'Blob', { value: OriginalBlob });
  globalThis.ResizeObserver = OriginalResizeObserver;
});

it('previews TSV fields containing commas as single cells', async () => {
  mockRequest.mockResolvedValue({
    data: new Blob(['city, region\tname\nParis, France\tLouvre']),
  });
  render(<CSVFileViewer url="/documents/tsv/preview" fileType="tsv" />);

  for (const text of ['city, region', 'Paris, France', 'Louvre']) {
    const matches = await screen.findAllByText(text);
    expect(matches.some((cell) => !cell.closest('[aria-hidden="true"]'))).toBe(
      true,
    );
  }
});
