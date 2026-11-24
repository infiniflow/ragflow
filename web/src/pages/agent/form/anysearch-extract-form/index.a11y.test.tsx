import { TooltipProvider } from '@/components/ui/tooltip';
import { act, render, screen } from '@testing-library/react';
import AnySearchExtractForm from './index';

let mockForm: any;

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('i18next', () => ({
  ...jest.requireActual('i18next'),
  t: (key: string) => key,
}));
jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('../../hooks/use-watch-form-change', () => ({
  useWatchFormChange: (_id: string | undefined, form: any) => {
    mockForm = form;
  },
}));
// The variable menu needs the Canvas store; keep the editor and its real
// Lexical contenteditable so form accessibility is tested at the DOM boundary.
jest.mock('../components/prompt-editor/variable-picker-plugin', () => ({
  __esModule: true,
  default: () => null,
}));

beforeAll(() => {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

it('associates the real URL editor with its label and validation message', async () => {
  const { container } = render(
    <TooltipProvider>
      <AnySearchExtractForm
        node={{ id: 'AnySearchExtract:0', data: { form: {} } } as any}
      />
    </TooltipProvider>,
  );
  const editor = container.querySelector('[contenteditable="true"]');
  expect(editor).toHaveAccessibleName('flow.anySearchExtractUrl');
  expect(editor).toHaveAttribute(
    'id',
    screen.getByText('flow.anySearchExtractUrl').closest('label')?.htmlFor,
  );
  expect(editor).toHaveAttribute('aria-invalid', 'false');

  await act(async () => {
    mockForm.setError('url', { message: 'URL required' });
  });
  expect(editor).toHaveAttribute('aria-invalid', 'true');
  expect(editor?.getAttribute('aria-describedby')?.split(' ')).toContain(
    screen.getByText('URL required').id,
  );
});
