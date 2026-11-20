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

import { OntologyDriftPanel } from './drift-panel';

// The panel's whole point is the evidence line, so the mock interpolates the
// way i18next does — returning the raw "{{count}}" would assert nothing.
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, options?: Record<string, unknown>) => {
      const template = (options?.defaultValue as string) ?? _key;
      return template.replace(/\{\{(\w+)\}\}/g, (_match, name: string) =>
        String(options?.[name] ?? ''),
      );
    },
  }),
}));

const insight = () => ({
  template_id: 'tpl-1',
  dataset_id: 'ds-1',
  version: 7,
  as_of: 1_700_000_000_000,
  stale: false,
  draft_pending: false,
  payload: {
    drift: {
      undeclared_classes: [
        {
          type: 'Invoice',
          entities: 18401,
          declared: false,
          fields: [
            {
              type: 'due_date',
              assertions: 0,
              instances: 18401,
              fill_rate: 0.61,
            },
          ],
        },
      ],
      unused_classes: [{ type: 'Warranty', entities: 0, declared: true }],
      unused_properties: [
        { type: 'ships_to', source: 'Vendor', target: 'Place' },
      ],
      classes: [
        {
          type: 'Contract',
          entities: 812,
          declared: true,
          fields: [
            { type: 'amount', assertions: 0, instances: 812, fill_rate: 0.12 },
            { type: 'status', assertions: 0, instances: 812, fill_rate: 0.97 },
          ],
        },
      ],
      total_entities: 21000,
      total_relations: 640,
      dropped_relations: 0,
      untyped_entities: 0,
      truncated: false,
    },
    questions: [],
    required: 0,
    satisfied: 0,
  },
});

const baseProps = {
  insight: insight(),
  loading: false,
  refreshing: false,
  onRefresh: jest.fn(),
};

describe('OntologyDriftPanel', () => {
  it('lists the three groups with their own counts', () => {
    render(<OntologyDriftPanel {...baseProps} />);

    expect(screen.getByText('Not declared (1)')).toBeInTheDocument();
    expect(
      screen.getByText('Declared, zero instances (2)'),
    ).toBeInTheDocument();
    expect(screen.getByText('Attribute fill problems (1)')).toBeInTheDocument();
  });

  it('explains each finding with one line of evidence', () => {
    render(<OntologyDriftPanel {...baseProps} />);

    // The weakest field is what names the gap, not the class name alone.
    expect(
      screen.getByText(
        'Field due_date filled in 61% · the class is not declared',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Declared, but none of the 21000 rows is one'),
    ).toBeInTheDocument();
    // Only the attributes below the line are listed; a healthy rate is not news.
    expect(screen.getByText('amount 12%')).toBeInTheDocument();
    expect(screen.queryByText(/status 97%/)).not.toBeInTheDocument();
  });

  it('locates the class and shows its rows when a finding is clicked', () => {
    const onLocate = jest.fn();
    render(
      <OntologyDriftPanel
        {...baseProps}
        onLocate={onLocate}
        samples={{
          className: 'Invoice',
          rows: [
            { id: '1', name: 'INV-2031 Apple', description: '2024-03-02' },
          ],
          total: 18401,
        }}
      />,
    );

    fireEvent.click(screen.getByText('Invoice'));
    expect(onLocate).toHaveBeenCalledWith('Invoice');
    expect(screen.getByText('INV-2031 Apple')).toBeInTheDocument();
    expect(screen.getByText('See these 18401')).toBeInTheDocument();
  });

  // The row knows which attributes the corpus fills, so Declare has to hand them
  // over — declaring the bare name would bring the same finding back.
  it('declares the class together with the attributes the corpus fills', () => {
    const onDeclare = jest.fn();
    render(<OntologyDriftPanel {...baseProps} onDeclare={onDeclare} />);

    fireEvent.click(screen.getByText('Declare Invoice'));
    expect(onDeclare).toHaveBeenCalledWith('Invoice', ['due_date']);
  });

  it('deletes a declared class or relation from the draft', () => {
    const onRemoveClass = jest.fn();
    const onRemoveProperty = jest.fn();
    render(
      <OntologyDriftPanel
        {...baseProps}
        onRemoveClass={onRemoveClass}
        onRemoveProperty={onRemoveProperty}
      />,
    );

    fireEvent.click(screen.getByText('Delete class'));
    expect(onRemoveClass).toHaveBeenCalledWith('Warranty');

    fireEvent.click(screen.getByText('Delete relation'));
    expect(onRemoveProperty).toHaveBeenCalledWith('ships_to');
  });

  // "Declared, zero instances" is usually an under-described class, not a wrong
  // one — so deletion is not the only write the row offers.
  it('offers a description fix for a class the corpus never produced', () => {
    const onDescribeClass = jest.fn();
    render(
      <OntologyDriftPanel {...baseProps} onDescribeClass={onDescribeClass} />,
    );

    const [describe] = screen.getAllByText('Describe');
    fireEvent.click(describe);
    expect(onDescribeClass).toHaveBeenCalledWith('Warranty');
  });

  // A fill problem has one fix per attribute and it needs an answer, so the
  // action appears only for the row the reader opened.
  it('offers a description fix for each attribute below the noise line', () => {
    const onDescribeProperty = jest.fn();
    render(
      <OntologyDriftPanel
        {...baseProps}
        onDescribeProperty={onDescribeProperty}
      />,
    );

    expect(screen.queryByText('Describe')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('Contract'));
    fireEvent.click(screen.getByText('Describe'));
    expect(onDescribeProperty).toHaveBeenCalledWith('Contract', 'amount');
  });

  it('gives the footer actions a subject only after a row is selected', () => {
    const onDismiss = jest.fn();
    const onReview = jest.fn();
    render(
      <OntologyDriftPanel
        {...baseProps}
        onDismiss={onDismiss}
        onReview={onReview}
      />,
    );

    const dismiss = screen.getByText('Ignore with a reason');
    const review = screen.getByText('Open review queue');
    expect(dismiss).toBeDisabled();
    expect(review).toBeDisabled();

    fireEvent.click(screen.getByText('Warranty'));
    expect(dismiss).toBeEnabled();
    fireEvent.click(dismiss);
    expect(onDismiss).toHaveBeenCalledWith('Warranty');
  });

  // A dismissal is recorded per class; writing a relation's name into that slot
  // would silence a class of the same name in this dataset.
  it('does not let a relation finding be dismissed', () => {
    const onDismiss = jest.fn();
    const onReview = jest.fn();
    render(
      <OntologyDriftPanel
        {...baseProps}
        onDismiss={onDismiss}
        onReview={onReview}
      />,
    );

    fireEvent.click(screen.getByText('Vendor → Place'));
    expect(screen.getByText('Ignore with a reason')).toBeDisabled();
    expect(screen.getByText('Open review queue')).toBeEnabled();
  });

  it('hides a dismissed class and lists it with its reason and a lift', () => {
    const onRestore = jest.fn();
    render(
      <OntologyDriftPanel
        {...baseProps}
        dismissedClasses={new Set(['Invoice'])}
        dismissals={[
          { className: 'Invoice', reason: 'invoices live in contracts' },
        ]}
        onRestore={onRestore}
      />,
    );

    expect(screen.getByText('Not declared (0)')).toBeInTheDocument();
    expect(screen.getByText('invoices live in contracts')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Lift'));
    expect(onRestore).toHaveBeenCalledWith('Invoice');
  });

  it('says the dataset has not been measured rather than showing empty groups', () => {
    render(<OntologyDriftPanel {...baseProps} insight={null} />);

    expect(
      screen.getByText(
        'This dataset has not been measured against the model yet. A check compares the compiled rows with the declaration.',
      ),
    ).toBeInTheDocument();
  });
});
