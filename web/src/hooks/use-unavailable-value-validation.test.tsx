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

import { renderHook } from '@testing-library/react';
import { z } from 'zod';
import {
  useUnavailableCompilationTemplateGroupFormSchema,
  useUnavailableModelFormSchema,
  useUnavailableValueFormSchema,
} from './use-unavailable-value-validation';

const mockUseModelValidIds = jest.fn();
const mockUseCompilationTemplateGroupValidIds = jest.fn();

jest.mock('@/hooks/use-llm-request', () => ({
  useModelValidIds: () => mockUseModelValidIds(),
}));

jest.mock('@/hooks/use-compilation-template-group-request', () => ({
  useCompilationTemplateGroupValidIds: () =>
    mockUseCompilationTemplateGroupValidIds(),
}));

const Schema = z.object({ llm_id: z.string() });

describe('useUnavailableValueFormSchema', () => {
  const renderValueHook = (isFetched: boolean) =>
    renderHook(() =>
      useUnavailableValueFormSchema(Schema, {
        fieldName: 'llm_id',
        validIds: new Set(['m1']),
        isFetched,
        message: 'custom.unavailable',
      }),
    );

  it('flags a persisted value missing from the valid ids', () => {
    const { result } = renderValueHook(true);

    const parsed = result.current.formSchema.safeParse({ llm_id: 'gone' });

    expect(parsed.success).toBe(false);
    if (!parsed.success) {
      expect(parsed.error.issues[0].path).toEqual(['llm_id']);
      expect(parsed.error.issues[0].message).toBe('custom.unavailable');
    }
  });

  it('accepts a valid id', () => {
    const { result } = renderValueHook(true);

    expect(result.current.formSchema.safeParse({ llm_id: 'm1' }).success).toBe(
      true,
    );
  });

  it('stays quiet while the list is loading', () => {
    const { result } = renderValueHook(false);

    expect(
      result.current.formSchema.safeParse({ llm_id: 'gone' }).success,
    ).toBe(true);
  });

  it('does not flag an empty value', () => {
    const { result } = renderValueHook(true);

    expect(result.current.formSchema.safeParse({ llm_id: '' }).success).toBe(
      true,
    );
  });
});

describe('useUnavailableModelFormSchema', () => {
  it('flags a persisted model missing from the current user models', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['m1']),
      isFetched: true,
    });
    const { result } = renderHook(() => useUnavailableModelFormSchema(Schema));

    const parsed = result.current.formSchema.safeParse({ llm_id: 'gone' });

    expect(parsed.success).toBe(false);
    if (!parsed.success) {
      expect(parsed.error.issues[0].path).toEqual(['llm_id']);
      expect(parsed.error.issues[0].message).toBe('common.modelUnavailable');
    }
  });

  it('accepts a usable model', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(['m1']),
      isFetched: true,
    });
    const { result } = renderHook(() => useUnavailableModelFormSchema(Schema));

    expect(result.current.formSchema.safeParse({ llm_id: 'm1' }).success).toBe(
      true,
    );
  });

  it('stays quiet while the model list is loading', () => {
    mockUseModelValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: false,
    });
    const { result } = renderHook(() => useUnavailableModelFormSchema(Schema));

    expect(
      result.current.formSchema.safeParse({ llm_id: 'gone' }).success,
    ).toBe(true);
  });
});

describe('useUnavailableCompilationTemplateGroupFormSchema', () => {
  const TemplateSchema = z.object({
    compilation_template_group_id: z.string(),
  });

  it('flags a template group the current user cannot resolve', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(['g1']),
      isFetched: true,
    });
    const { result } = renderHook(() =>
      useUnavailableCompilationTemplateGroupFormSchema(TemplateSchema),
    );

    const parsed = result.current.formSchema.safeParse({
      compilation_template_group_id: 'gone',
    });

    expect(parsed.success).toBe(false);
    if (!parsed.success) {
      expect(parsed.error.issues[0].path).toEqual([
        'compilation_template_group_id',
      ]);
      expect(parsed.error.issues[0].message).toBe(
        'knowledgeConfiguration.compilationTemplateUnavailable',
      );
    }
  });

  it('accepts a resolvable template group and stays quiet while loading', () => {
    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(['g1']),
      isFetched: true,
    });
    const { result, rerender } = renderHook(() =>
      useUnavailableCompilationTemplateGroupFormSchema(TemplateSchema),
    );

    expect(
      result.current.formSchema.safeParse({
        compilation_template_group_id: 'g1',
      }).success,
    ).toBe(true);

    mockUseCompilationTemplateGroupValidIds.mockReturnValue({
      validIds: new Set(),
      isFetched: false,
    });
    rerender();

    expect(
      result.current.formSchema.safeParse({
        compilation_template_group_id: 'gone',
      }).success,
    ).toBe(true);
  });
});
