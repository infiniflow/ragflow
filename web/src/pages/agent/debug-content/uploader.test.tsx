import { useUploadAgentFile } from '@/hooks/use-agent-request';
import { act, fireEvent, render, waitFor } from '@testing-library/react';
import * as React from 'react';

import { FileUploadDirectUpload } from './uploader';

jest.mock('@/hooks/use-agent-request', () => ({
  useUploadAgentFile: jest.fn(),
}));

jest.mock('sonner', () => ({ toast: jest.fn() }));

jest.mock('@/components/ui/button', () => {
  const React = jest.requireActual<typeof import('react')>('react');
  return {
    Button: React.forwardRef<
      HTMLButtonElement,
      JSX.IntrinsicElements['button'] & {
        variant?: string;
        size?: string;
      }
    >(function MockButton({ variant, size, ...props }, ref) {
      return (
        <button {...props} data-variant={variant} data-size={size} ref={ref} />
      );
    }),
  };
});

type UploadedFile = { id: string; name: string };
type UploadResponse = { code: number; data?: UploadedFile };

const UploadAgentFile = jest.fn<Promise<UploadResponse>, [File[]]>();

/** Creates a controllable response so tests choose upload completion order without timers. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

/** Renders the real uploader/store with per-File requests and observes emitted form values. */
function setup(
  files: File[],
  options: { maxFiles?: number; value?: UploadedFile[] } = {},
) {
  const requests = new Map(
    files.map((file) => [file, deferred<UploadResponse>()]),
  );
  UploadAgentFile.mockImplementation(([file]) => requests.get(file)!.promise);
  const onChange = jest.fn();
  const view = render(
    <FileUploadDirectUpload
      value={options.value ?? []}
      onChange={onChange}
      maxFiles={options.maxFiles}
    />,
  );
  const input =
    view.container.querySelector<HTMLInputElement>('input[type="file"]')!;

  /** Selects files through the input and waits for the expected upload requests to start. */
  async function select(selectedFiles: File[], expectedUploads: number) {
    fireEvent.change(input, { target: { files: selectedFiles } });
    await waitFor(() =>
      expect(UploadAgentFile).toHaveBeenCalledTimes(expectedUploads),
    );
  }

  /** Resolves this File's current request and flushes the resulting React updates. */
  async function complete(file: File, data: UploadedFile) {
    await act(async () => {
      requests.get(file)!.resolve({ code: 0, data });
    });
  }

  /** Clicks an item's real delete control to exercise the store's selection callback. */
  function remove(index: number) {
    const item = view.getAllByRole('listitem')[index];
    fireEvent.click(
      item.querySelector<HTMLButtonElement>(
        '[data-slot="file-upload-item-delete"]',
      )!,
    );
  }

  return { ...view, input, onChange, requests, select, complete, remove };
}

describe('Agent file upload identity', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.mocked(useUploadAgentFile).mockReturnValue({
      uploadAgentFile: UploadAgentFile,
    } as never);
  });

  it('removes the matching response when uploads complete out of order', async () => {
    const a = new File(['a'], 'A.pdf');
    const b = new File(['b'], 'B.pdf');
    const uploadA = { id: 'upload-A', name: a.name };
    const uploadB = { id: 'upload-B', name: b.name };
    const view = setup([a, b]);

    await view.select([a, b], 2);
    await view.complete(b, uploadB);
    await view.complete(a, uploadA);
    view.remove(0);

    expect(view.queryByText(a.name)).not.toBeInTheDocument();
    expect(view.getByText(b.name)).toBeInTheDocument();
    expect(view.onChange).toHaveBeenLastCalledWith([uploadB]);
  });

  it.each(['response', 'rejection'])(
    'keeps a successful upload when removing a failed file (%s)',
    async (failure) => {
      const a = new File(['a'], 'A.pdf');
      const b = new File(['b'], 'B.pdf');
      const uploadB = { id: 'upload-B', name: b.name };
      const view = setup([a, b]);

      await view.select([a, b], 2);
      await act(async () => {
        if (failure === 'response') {
          view.requests.get(a)!.resolve({ code: 1 });
        } else {
          view.requests.get(a)!.reject(new Error('Network error'));
        }
      });
      await view.complete(b, uploadB);
      view.remove(0);

      expect(view.onChange).toHaveBeenLastCalledWith([uploadB]);
      expect(view.getByText(b.name)).toBeInTheDocument();
    },
  );

  it('ignores an upload that completes after its file was removed', async () => {
    const a = new File(['a'], 'A.pdf');
    const b = new File(['b'], 'B.pdf');
    const uploadB = { id: 'upload-B', name: b.name };
    const view = setup([a, b]);

    await view.select([a, b], 2);
    view.remove(0);
    await view.complete(b, uploadB);
    const changeCount = view.onChange.mock.calls.length;
    await view.complete(a, { id: 'removed-A', name: a.name });

    expect(view.onChange).toHaveBeenCalledTimes(changeCount);
    expect(view.onChange).toHaveBeenLastCalledWith([uploadB]);
    expect(view.queryByText(a.name)).not.toBeInTheDocument();
  });

  it('distinguishes different files with the same name', async () => {
    const a = new File(['first'], 'report.pdf');
    const b = new File(['second'], 'report.pdf');
    const uploadB = { id: 'upload-B', name: b.name };
    const view = setup([a, b]);

    await view.select([a, b], 2);
    await view.complete(b, uploadB);
    await view.complete(a, { id: 'upload-A', name: a.name });
    view.remove(0);

    expect(view.getAllByRole('listitem')).toHaveLength(1);
    expect(view.onChange).toHaveBeenLastCalledWith([uploadB]);
  });

  it('ignores the previous request when the same file is removed and re-added', async () => {
    const file = new File(['a'], 'A.pdf');
    const view = setup([file]);

    await view.select([file], 1);
    const previousRequest = view.requests.get(file)!;
    view.remove(0);
    view.requests.set(file, deferred<UploadResponse>());
    await view.select([file], 2);
    const uploadedFile = { id: 'new-upload', name: file.name };
    await view.complete(file, uploadedFile);
    await act(async () => {
      previousRequest.resolve({
        code: 0,
        data: { id: 'removed-upload', name: file.name },
      });
    });

    expect(view.onChange).toHaveBeenLastCalledWith([uploadedFile]);
  });

  it('supports single-file uploads without dropping initial form values', async () => {
    const file = new File(['a'], 'A.pdf');
    const existing = { id: 'existing-upload', name: 'existing.pdf' };
    const uploadedFile = { id: 'new-upload', name: file.name };
    const view = setup([file], { maxFiles: 1, value: [existing] });

    expect(view.input.multiple).toBe(false);
    await view.select([file], 1);
    await view.complete(file, uploadedFile);
    expect(view.onChange).toHaveBeenLastCalledWith([existing, uploadedFile]);
    view.remove(0);

    expect(view.onChange).toHaveBeenLastCalledWith([existing]);
  });
});
