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

import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import English from '@/locales/en';
import Portuguese from '@/locales/pt-br';
import {
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from '@testing-library/react';
import { createInstance } from 'i18next';
import { PropsWithChildren } from 'react';
import { useForm } from 'react-hook-form';
import { I18nextProvider } from 'react-i18next';
import {
  CrossLanguageFormField,
  useCrossLanguageOptions,
} from './cross-language-form-field';

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollIntoView = () => {};
});

/**
 * Creates an isolated i18n instance for the requested locale so tests do not
 * share language state, using English as the fallback for untranslated labels.
 */
async function createWrapper(language: string) {
  const i18n = createInstance();
  await i18n.init({
    lng: language,
    fallbackLng: 'en',
    resources: { en: English, 'pt-BR': Portuguese },
    interpolation: { escapeValue: false },
  });
  /** Supplies the translation and tooltip contexts required by the field. */
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <I18nextProvider i18n={i18n}>
        <TooltipProvider>{children}</TooltipProvider>
      </I18nextProvider>
    );
  }
  return Wrapper;
}

/**
 * Exposes live form values for either the nested prompt field or an explicit
 * top-level field, starting with English selected to check selection retention.
 */
function Harness({ name }: { name?: string }) {
  const form = useForm({
    defaultValues: {
      cross_languages: ['English'],
      prompt_config: { cross_languages: ['English'] },
    },
  });
  const values = form.watch(
    name ? 'cross_languages' : 'prompt_config.cross_languages',
  );
  return (
    <Form {...form}>
      <CrossLanguageFormField name={name} />
      <output data-testid="languages">{JSON.stringify(values)}</output>
    </Form>
  );
}

describe('cross-language Portuguese support', () => {
  it('preserves Slovak and Czech alongside Portuguese after merging upstream', async () => {
    const wrapper = await createWrapper('en');
    const { result } = renderHook(() => useCrossLanguageOptions(), { wrapper });
    expect(result.current).toEqual(
      expect.arrayContaining([
        { label: 'Slovak', value: 'Slovak' },
        { label: 'Czech', value: 'Czech' },
        { label: 'Portuguese', value: 'Portuguese' },
      ]),
    );
    const values = result.current.map(({ value }) => value);
    expect(new Set(values).size).toBe(values.length);
  });

  it.each([
    ['en', 'Portuguese'],
    ['pt-BR', 'Português'],
  ])(
    'localizes the label in %s while preserving the API value',
    async (language, label) => {
      const wrapper = await createWrapper(language);
      const { result } = renderHook(() => useCrossLanguageOptions(), {
        wrapper,
      });
      expect(result.current).toContainEqual({ label, value: 'Portuguese' });
    },
  );

  it.each([undefined, 'cross_languages'])(
    'selects and deselects Portuguese without changing English for field %s',
    async (name) => {
      const wrapper = await createWrapper('pt-BR');
      render(<Harness name={name} />, { wrapper });
      const trigger = screen.getByRole('button', { name: 'Inglês' });
      fireEvent.click(trigger);
      fireEvent.click(screen.getByRole('option', { name: 'Português' }));
      expect(screen.getByTestId('languages')).toHaveTextContent(
        '["English","Portuguese"]',
      );
      expect(within(trigger).getByText('Português')).toBeInTheDocument();
      fireEvent.click(screen.getByRole('option', { name: 'Português' }));
      expect(screen.getByTestId('languages')).toHaveTextContent('["English"]');
    },
  );
});
