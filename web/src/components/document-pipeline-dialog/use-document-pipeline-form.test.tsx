import { renderHook, act } from '@testing-library/react';
import React from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-pipeline-operator', () => ({
  useActiveTab: () => ({ activeTab: '', setActiveTab: jest.fn() }),
  usePipelineOperatorNodes: () => ({ operatorNodes: [], loading: false }),
  useResetParserConfigOnPipelineChange: () => {},
}));

// Cut the chain parser-config-validation -> compilation-form -> locales/config
// (which calls i18n.use(initReactI18next) and would crash under the mock).
jest.mock('@/components/pipeline-operator-tabs/parser-config-validation', () => ({
  addParserConfigIssues: jest.fn(),
}));

import { useDocumentPipelineForm } from './use-document-pipeline-form';

function renderForm(props: any) {
  return renderHook((p: any) => useDocumentPipelineForm(p), { initialProps: props });
}

describe('useDocumentPipelineForm pipeline_version_id default (design §8: never show Latest version)', () => {
  it('defaults to the latest concrete version when the document is unpinned (R1a)', () => {
    const { result } = renderForm({
      parserId: 'naive',
      pipelineId: undefined,
      pipelineVersionId: undefined,
      latestVersionId: 'v2',
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('v2');
  });

  it('keeps an explicitly pinned version instead of overriding to latest (R1b)', () => {
    const { result } = renderForm({
      parserId: 'naive',
      pipelineId: 'canvas-1',
      pipelineVersionId: 'v1',
      latestVersionId: 'v2',
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('v1');
  });

  it('stays empty when there is no version list at all (R1c)', () => {
    const { result } = renderForm({
      parserId: 'naive',
      pipelineId: undefined,
      pipelineVersionId: undefined,
      latestVersionId: undefined,
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('');
  });

  it('adopts the latest version once the list loads after mount (R1d, async)', () => {
    const { result, rerender } = renderForm({
      parserId: 'naive',
      pipelineId: undefined,
      pipelineVersionId: undefined,
      latestVersionId: undefined,
    });
    expect(result.current.form.getValues('pipeline_version_id')).toBe('');
    rerender({
      parserId: 'naive',
      pipelineId: undefined,
      pipelineVersionId: undefined,
      latestVersionId: 'v3',
    });
    act(() => {});
    expect(result.current.form.getValues('pipeline_version_id')).toBe('v3');
  });
});
