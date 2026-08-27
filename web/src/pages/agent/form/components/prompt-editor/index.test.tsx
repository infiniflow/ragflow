import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

jest.mock('@lexical/code', () => ({
  CodeHighlightNode: class CodeHighlightNode {},
  CodeNode: class CodeNode {},
}));

jest.mock('@lexical/react/LexicalComposer', () => ({
  LexicalComposer: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock('@lexical/react/LexicalComposerContext', () => ({
  useLexicalComposerContext: () => [{ update: jest.fn() }],
}));

jest.mock('@lexical/react/LexicalContentEditable', () => ({
  ContentEditable: ({ className }: { className?: string }) => (
    <div data-testid="content-editable" className={className} />
  ),
}));

jest.mock('@lexical/react/LexicalErrorBoundary', () => ({
  LexicalErrorBoundary: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock('@lexical/react/LexicalRichTextPlugin', () => ({
  RichTextPlugin: ({
    contentEditable,
    placeholder,
  }: {
    contentEditable: React.ReactNode;
    placeholder: React.ReactNode;
  }) => (
    <>
      {contentEditable}
      {placeholder}
    </>
  ),
}));

jest.mock('@lexical/rich-text', () => ({
  HeadingNode: class HeadingNode {},
  QuoteNode: class QuoteNode {},
}));

jest.mock('lexical', () => ({
  $getRoot: jest.fn(),
  $getSelection: jest.fn(),
}));

jest.mock('@/components/ui/switch', () => ({
  Switch: () => <button type="button" />,
}));

jest.mock('@/components/ui/tooltip', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./enter-key-plugin', () => ({
  EnterKeyPlugin: () => null,
}));

jest.mock('./paste-handler-plugin', () => ({
  PasteHandlerPlugin: () => null,
}));

jest.mock('./variable-node', () => ({
  VariableNode: class VariableNode {},
}));

jest.mock('./variable-on-change-plugin', () => ({
  VariableOnChangePlugin: () => null,
}));

jest.mock('./variable-picker-plugin', () => ({
  __esModule: true,
  default: () => null,
}));

import { PromptEditor } from './index';

describe('PromptEditor placeholder layout', () => {
  it.each([
    { name: 'defaults to a toolbar offset', props: {}, classes: ['top-12'] },
    {
      name: 'keeps the toolbar offset when toolbar is explicit',
      props: { showToolbar: true },
      classes: ['top-12'],
    },
    {
      name: 'uses the compact offset without a toolbar',
      props: { showToolbar: false },
      classes: ['top-1'],
    },
    {
      name: 'truncates single-line placeholders',
      props: { multiLine: false },
      classes: ['top-12', 'truncate', 'max-w-[calc(100%-4rem)]'],
    },
  ])('$name', ({ props, classes }) => {
    render(<PromptEditor placeholder="Prompt placeholder" {...props} />);

    const placeholder = screen.getByText('Prompt placeholder');
    for (const className of classes) {
      expect(placeholder).toHaveClass(className);
    }
  });
});
