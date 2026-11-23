/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { fireEvent, render, screen } from '@testing-library/react';
import TestingForm from './testing-form';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) =>
      key === 'knowledgeDetails.testIntegerValidation' ? '请输入整数' : key,
  }),
}));

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'dataset-1' }),
}));

jest.mock('../contexts/knowledge-base-context', () => ({
  useOwnerTenantId: () => 'tenant-1',
}));

jest.mock('@/components/cross-language-form-field', () => ({
  CrossLanguageFormField: () => null,
}));

jest.mock('@/components/form-container', () => ({
  FormContainer: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock('@/components/metadata-filter', () => ({
  MetadataFilter: () => null,
  MetadataFilterSchema: {},
}));

jest.mock('@/components/rerank', () => ({
  RerankFormFields: () => null,
}));

jest.mock('@/components/similarity-slider', () => ({
  SimilaritySliderFormField: () => null,
  initialSimilarityThresholdValue: {},
  initialKeywordsSimilarityWeightValue: {},
  similarityThresholdSchema: {},
  keywordsSimilarityWeightSchema: {},
}));

jest.mock('@/components/ui/dual-range-slider', () => {
  const { forwardRef } = jest.requireActual<typeof import('react')>('react');
  return {
    SingleFormSlider: forwardRef<HTMLDivElement, Record<string, unknown>>(
      (_props, ref) => <div ref={ref} />,
    ),
  };
});

jest.mock('@/components/ui/tooltip', () => ({
  FormTooltip: () => null,
}));

test('shows localized inline errors instead of native validation for decimals', async () => {
  const refetch = jest.fn();
  const { container } = render(
    <TestingForm loading={false} refetch={refetch} setValues={jest.fn()} />,
  );

  expect(container.querySelector('form')).toHaveAttribute('novalidate');

  const [rerankCandidatesInput, topNInput] = screen.getAllByRole('spinbutton');
  fireEvent.change(rerankCandidatesInput, { target: { value: '64.99' } });
  fireEvent.change(topNInput, { target: { value: '10.88' } });
  fireEvent.change(screen.getByRole('textbox'), {
    target: { value: '红楼梦' },
  });
  fireEvent.click(screen.getByRole('button', { name: /testingLabel/ }));

  expect(await screen.findAllByText('请输入整数')).toHaveLength(2);
  expect(refetch).not.toHaveBeenCalled();
});
