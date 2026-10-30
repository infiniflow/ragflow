import { renderHook, act } from '@testing-library/react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-pipeline-operator', () => ({
  useActiveTab: () => ({ activeTab: '', setActiveTab: jest.fn() }),
  usePipelineOperatorNodes: () => ({ operatorNodes: [], loading: false }),
  useResetParserConfigOnPipelineChange: () => {},
}));

// The hook now fetches the canvas version list itself and derives the latest
// id reactively, so we mock the fetch and drive it via a mutable array.
let mockVersions: Array<{ id: string; version?: number; name?: string }> = [];
jest.mock('@/hooks/use-agent-request', () => ({
  useFetchCanvasVersionList: () => ({
    data: mockVersions,
    loading: false,
    isError: false,
  }),
  // The three hooks below back useParserTreeData, which the form hook
  // consults for the missing-option validation.
  useFetchBuiltinPipelines: () => ({ options: [], loading: false }),
  useFetchAgentList: () => ({ data: { canvas: [], total: 0 }, loading: false }),
  useFetchCanvasVersions: (canvasIds: string[]) =>
    canvasIds.map(() => ({ isSuccess: true, data: mockVersions })),
}));

// Cut the chain parser-config-validation -> compilation-form -> locales/config
// (which calls i18n.use(initReactI18next) and would crash under the mock).
jest.mock(
  '@/components/pipeline-operator-tabs/parser-config-validation',
  () => ({
    addParserConfigIssues: jest.fn(),
  }),
);

import { useDocumentPipelineForm } from './use-document-pipeline-form';

function renderForm(props: any) {
  return renderHook((p: any) => useDocumentPipelineForm(p), {
    initialProps: props,
  });
}

describe('useDocumentPipelineForm pipeline_version_id default (design S8: never show Latest version)', () => {
  beforeEach(() => {
    mockVersions = [];
  });

  it('adopts the latest concrete version for an unpinned document (R1a)', () => {
    mockVersions = [{ id: 'v2', version: 2, name: 'v2' }];
    const { result } = renderForm({
      parserId: 'naive',
      pipelineId: 'canvas-1',
      pipelineVersionId: undefined,
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('v2');
  });

  it('keeps an explicitly pinned version instead of overriding to latest (R1b)', () => {
    mockVersions = [{ id: 'v2', version: 2, name: 'v2' }];
    const { result } = renderForm({
      parserId: 'naive',
      pipelineId: 'canvas-1',
      pipelineVersionId: 'v1',
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('v1');
  });

  it('stays empty when there is no version list at all (R1c)', () => {
    mockVersions = [];
    const { result } = renderForm({
      parserId: 'naive',
      pipelineId: undefined,
      pipelineVersionId: undefined,
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('');
  });

  it('adopts the latest version once the list loads after mount (R1d, async)', () => {
    mockVersions = [];
    const { result, rerender } = renderForm({
      parserId: 'naive',
      pipelineId: 'canvas-1',
      pipelineVersionId: undefined,
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('');
    mockVersions = [{ id: 'v3', version: 3, name: 'v3' }];
    act(() => {
      rerender({
        parserId: 'naive',
        pipelineId: 'canvas-1',
        pipelineVersionId: undefined,
      });
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('v3');
  });
});
