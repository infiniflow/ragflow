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

import {
  CompetencyQuestionDialog,
  CompetencyQuestionsPanel,
} from './competency-questions';

// `any` on purpose: this file contains jest.mock, so it goes through
// esbuild-jest's babel pass, which cannot strip imported bindings used as type
// annotations.
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: any) => {
      if (!options) return key;
      const template: string = options.defaultValue ?? key;
      return template.replace(
        /{{\s*(\w+)\s*}}/g,
        (_match: string, name: string) => String(options[name] ?? ''),
      );
    },
  }),
}));

const vocabulary: any = {
  classes: [
    { type: 'contract', terms: ['contract', '合同'] },
    { type: 'vendor', terms: ['vendor', '供应商'] },
  ],
  properties: [
    {
      type: 'amount',
      object: false,
      terms: ['amount', '金额'],
      domains: ['contract'],
      ranges: [],
    },
  ],
};

const renderDialog = (props: any = {}) =>
  render(
    <CompetencyQuestionDialog
      open
      onOpenChange={() => undefined}
      existing={[]}
      vocabulary={vocabulary}
      onSubmit={() => undefined}
      saving={false}
      {...props}
    />,
  );

const questionBox = () => screen.getAllByRole('textbox')[1];
const elementsBox = () =>
  screen.getAllByRole('textbox')[2] as HTMLTextAreaElement;
const saveButton = () =>
  screen
    .getAllByRole('button')
    .find((button) => button.textContent === 'Save')!;

describe('CompetencyQuestionDialog element derivation', () => {
  it('fills the element list while the question is typed', () => {
    renderDialog();

    fireEvent.change(questionBox(), {
      target: { value: '每份合同的金额是多少？' },
    });

    // Derived, not typed: the author never touches the clause box.
    expect(elementsBox().value).toContain('class:contract');
    expect(elementsBox().value).toContain('property:contract.amount');
    expect(saveButton()).not.toBeDisabled();
  });

  it('keeps saving disabled when nothing can be derived', () => {
    renderDialog();

    fireEvent.change(questionBox(), {
      target: { value: '这批货物的仓储温度是多少？' },
    });

    expect(elementsBox().value).toBe('');
    // An empty list would make the backend report the question as answered, so
    // the dialog refuses to submit it.
    expect(saveButton()).toBeDisabled();
  });

  it('re-derives a list saved by hand when the question changes', () => {
    // The list predates the derivation, so it is history rather than an edit:
    // changing the question must replace it instead of staying locked.
    renderDialog({
      initial: {
        type: 'CQ-1',
        question: '每份合同的金额是多少？',
        elements: 'class:vendor',
      },
    });

    expect(elementsBox().value).toBe('class:vendor');

    fireEvent.change(questionBox(), {
      target: { value: '每份合同由哪个供应商签订？' },
    });

    expect(elementsBox().value).toContain('class:contract');
    expect(elementsBox().value).toContain('class:vendor');
  });

  it('offers the declared elements as a pick list when nothing matches', () => {
    renderDialog();

    // Chinese question against English declarations: tracing finds nothing, so
    // the dialog must still give the author a way forward that is not "retype
    // your question".
    fireEvent.change(questionBox(), {
      target: { value: '这批货物的仓储温度是多少？' },
    });
    expect(elementsBox().value).toBe('');

    fireEvent.click(screen.getByRole('button', { name: 'contract' }));

    expect(elementsBox().value).toContain('class:contract');
    expect(saveButton()).not.toBeDisabled();
  });

  it('stops overwriting once a clause is edited in this session', () => {
    renderDialog();

    fireEvent.change(questionBox(), {
      target: { value: '每份合同由谁签订？' },
    });
    fireEvent.change(elementsBox(), { target: { value: 'class:vendor' } });
    fireEvent.change(questionBox(), {
      target: { value: '每份合同的金额是多少？' },
    });

    expect(elementsBox().value).toBe('class:vendor');
  });
});

const panelInsight: any = {
  template_id: 't1',
  dataset_id: 'd1',
  version: 7,
  as_of: 1763600000000,
  stale: false,
  draft_pending: false,
  payload: {
    drift: {},
    required: 1,
    satisfied: 0,
    questions: [
      {
        id: 'CQ-1',
        question: '每份合同的金额是多少？',
        required: 1,
        satisfied: 0,
        answerable: false,
        elements: [
          {
            kind: 'class',
            name: 'contract',
            required: true,
            satisfied: false,
            observed: 0,
            declared: true,
            reason: 'not_extracted',
          },
        ],
      },
    ],
  },
};

describe('CompetencyQuestionsPanel actions', () => {
  const renderPanel = (props: any = {}) =>
    render(
      <CompetencyQuestionsPanel
        questions={[
          {
            type: 'CQ-1',
            question: '每份合同的金额是多少？',
            elements: 'class:contract',
          },
        ]}
        insight={panelInsight}
        loading={false}
        onEdit={() => undefined}
        onDelete={() => undefined}
        onCreate={() => undefined}
        measuredLabel="v7 · 2026-11-20"
        {...props}
      />,
    );

  it('locates the element the corpus failed to fill', () => {
    const onLocate = jest.fn();
    renderPanel({ onLocate });

    fireEvent.click(
      screen.getByRole('button', { name: 'Locate contract in canvas' }),
    );

    expect(onLocate).toHaveBeenCalledWith('class:contract');
  });

  it('runs a measurement and names the snapshot it describes', () => {
    const onRunCheck = jest.fn();
    renderPanel({ onRunCheck });

    fireEvent.click(
      screen.getByRole('button', { name: 'Run and show evidence' }),
    );
    expect(onRunCheck).toHaveBeenCalled();
    expect(screen.getByText(/Measured on v7/)).toBeInTheDocument();
  });
});
