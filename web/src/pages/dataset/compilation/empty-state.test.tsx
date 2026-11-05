import { render, screen } from '@testing-library/react';
import { createInstance } from 'i18next';
import { I18nextProvider } from 'react-i18next';

import { GenerateStatus } from '@/constants/knowledge';
import En from '@/locales/en';
import Zh from '@/locales/zh';

import { ViewMode } from './constants';
import { CompilationEmptyState } from './empty-state';

const mockCompileStatus = { value: GenerateStatus.Start };

jest.mock('@/hooks/use-dataset-generate', () => ({
  useGenerateStatus: () => ({ status: mockCompileStatus.value }),
}));

jest.mock('./compile-progress-board', () => ({
  CompileProgressBoard: () => <div data-testid="compile-progress" />,
}));

async function renderWithLocale(element: JSX.Element, language = 'en') {
  const i18n = createInstance();
  await i18n.init({ lng: language, resources: { en: En, zh: Zh } });
  render(<I18nextProvider i18n={i18n}>{element}</I18nextProvider>);
}

describe('CompilationEmptyState', () => {
  beforeEach(() => {
    mockCompileStatus.value = GenerateStatus.Start;
  });

  it.each([
    ['en', En],
    ['zh', Zh],
  ] as const)('explains Wiki setup in %s', async (language, resources) => {
    await renderWithLocale(
      <CompilationEmptyState type={ViewMode.LlmWiki} />,
      language,
    );

    const copy = resources.translation.knowledgeCompilation;
    expect(
      screen.getByText(copy.compilationPipelineRequired),
    ).toBeInTheDocument();
    expect(screen.getByText(copy.wikiTemplateRequired)).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: copy.compilationGuide }),
    ).toHaveAttribute(
      'href',
      'https://ragflow.io/docs/knowledge_compilation/apply_knowledge_compilation_template',
    );
  });

  it.each([
    ViewMode.Skills,
    ViewMode.Graph,
    ViewMode.MindMap,
    ViewMode.Timeline,
  ])('keeps Wiki-specific requirements out of %s setup', async (type) => {
    await renderWithLocale(<CompilationEmptyState type={type} />);

    const copy = En.translation.knowledgeCompilation;
    expect(
      screen.getByText(copy.compilationPipelineRequired),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(copy.wikiTemplateRequired),
    ).not.toBeInTheDocument();
  });

  it.each([GenerateStatus.Running, GenerateStatus.Failed])(
    'keeps the progress board visible when compilation is %s',
    async (status) => {
      mockCompileStatus.value = status;
      await renderWithLocale(<CompilationEmptyState type={ViewMode.LlmWiki} />);

      expect(screen.getByTestId('compile-progress')).toBeInTheDocument();
      expect(screen.queryByRole('link')).not.toBeInTheDocument();
    },
  );
});
