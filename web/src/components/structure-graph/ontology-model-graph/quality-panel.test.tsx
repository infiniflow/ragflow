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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { OntologyQualityPanel } from './quality-panel';

// `any` on purpose: this file contains jest.mock, so it goes through
// esbuild-jest's babel pass, which cannot strip imported bindings used as type
// annotations (the same constraint noted in detail-panel.test.tsx).
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    // Interpolates {{placeholders}}, so an assertion cannot pass on a raw
    // template no user ever sees.
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

const graphWith = (quality: any): any => ({
  classes: [
    { type: 'person', declared: true, entities: 0 },
    { type: 'place', declared: true, entities: 3 },
  ],
  properties: [
    { type: 'born_in', source: 'person', target: 'place', relations: 0, declared: true },
    { type: 'invented', source: 'place', target: 'place', relations: 2, declared: false },
  ],
  pitfalls: [],
  quality,
});

const baseQuality = (overrides: any = {}): any => ({
  undeclared_properties: 0,
  dropped_relations: 0,
  untyped_entities: 0,
  classes_without_instances: 0,
  properties_without_assertions: 0,
  pitfalls: 0,
  samples_truncated: false,
  ...overrides,
});

/** Renders the ledger and opens it, which is how a reader reaches any subject. */
const renderExpanded = (graph: any, props: any = {}) => {
  render(<OntologyQualityPanel graph={graph} {...props} />);
  fireEvent.click(screen.getByTestId('ontology-quality-toggle'));
};

describe('OntologyQualityPanel', () => {
  // Nothing wired to the ledger renders nothing: every host that shows the
  // ontology view gets the panel only when the backend sent the ledger.
  it('renders nothing without a ledger', () => {
    const { container } = render(<OntologyQualityPanel graph={undefined} />);
    expect(container.firstChild).toBeNull();
  });

  // The ontology view exists to be read as a graph, so the ledger keeps one line
  // until it is asked for the details — a permanently expanded list of thirty
  // subjects is a screen of text with no room left for the canvas.
  it('keeps one line until the details are asked for', () => {
    render(
      <OntologyQualityPanel
        graph={graphWith(baseQuality({ undeclared_properties: 1 }))}
      />,
    );

    const line = screen.getByTestId('ontology-quality-line');
    expect(line.textContent).toContain('1 undeclared');
    expect(line.textContent).toContain('never exercised');
    expect(screen.queryByTestId('ontology-quality-details')).toBeNull();
    expect(screen.queryByText('invented')).toBeNull();

    fireEvent.click(screen.getByTestId('ontology-quality-toggle'));
    expect(screen.getByTestId('ontology-quality-details')).toBeTruthy();
    expect(screen.getByText('invented')).toBeTruthy();
  });

  // Every row is present once opened, including the ones at zero: a row that
  // disappears at zero tells the reader nothing about whether it was checked.
  it('shows every row once opened, including the ones at zero', () => {
    renderExpanded(graphWith(baseQuality()));
    ['undeclared', 'dropped', 'untyped', 'pitfalls', 'coverage'].forEach(
      (key) => {
        expect(screen.getByTestId(`ontology-quality-${key}`)).toBeTruthy();
      },
    );
  });

  // Five rejected assertions of one property are ONE edit. Listing them as five
  // sentences leaves the reader to work out that the fix is a single field, and
  // that is the difference between a report and a work queue.
  it('collapses the rejections of one property into a single edit', () => {
    const samples = [1, 2, 3, 4, 5].map((at) => ({
      property: 'part_of',
      from: `Season ${at} of You`,
      to: 'You',
      from_type: 'event',
      to_type: 'work',
      reason: 'dropped part_of: the template declares range place|event, the assertion used "work"',
      doc_id: 'doc-1',
    }));
    const graph = graphWith(
      baseQuality({ dropped_relations: 5, dropped_samples: samples }),
    );
    graph.properties = [
      { type: 'part_of', source: 'event', target: 'place', relations: 3, declared: true },
    ];
    renderExpanded(graph);

    const actions = screen.getAllByTestId(/^ontology-quality-action-/);
    expect(actions).toHaveLength(1);
    expect(actions[0].textContent).toContain('Add “work” to part_of’s range');
    expect(actions[0].textContent).toContain('declared: place');
    expect(actions[0].textContent).toContain('covers 5 assertion(s)');
  });

  // Selecting is separate from applying, and applying is one batch: the reader
  // marks every line they agree with, then compiles once. Five lines must not
  // mean five compiles, because a compile calls the model.
  it('applies every selected edit as one batch, on one click', async () => {
    const onApplyFixes = jest.fn().mockResolvedValue(undefined);
    const graph = graphWith(
      baseQuality({
        undeclared_properties: 1,
        dropped_relations: 2,
        dropped_samples: [
          {
            property: 'part_of',
            from: 'Season 1 of You',
            to: 'You',
            from_type: 'event',
            to_type: 'work',
            doc_id: 'doc-1',
          },
          {
            property: 'part_of',
            from: 'Season 2 of You',
            to: 'You',
            from_type: 'event',
            to_type: 'work',
            doc_id: 'doc-2',
          },
        ],
      }),
    );
    graph.properties = [
      { type: 'part_of', source: 'event', target: 'place', relations: 3, declared: true },
      { type: 'invented', source: 'place', target: 'place', relations: 2, declared: false },
    ];
    renderExpanded(graph, { onApplyFixes });

    // Nothing selected yet: the batch button exists but cannot start a compile.
    const batch = screen.getByTestId('ontology-quality-apply-batch') as HTMLButtonElement;
    expect(batch.disabled).toBe(true);

    fireEvent.click(
      screen.getByTestId('ontology-quality-select-widen|part_of|range|work'),
    );
    fireEvent.click(screen.getByTestId('ontology-quality-select-declare|invented'));
    expect(batch.disabled).toBe(false);

    // One click applies: a confirm step that can be interrupted, or reset by a
    // re-render, is what makes a deliberate selection write nothing.
    fireEvent.click(batch);

    expect(onApplyFixes).toHaveBeenCalledTimes(1);
    expect(onApplyFixes).toHaveBeenCalledWith([
      {
        op: 'widen',
        property: 'part_of',
        side: 'range',
        class: 'work',
        docIds: ['doc-1', 'doc-2'],
      },
      {
        op: 'declare',
        property: 'invented',
        domain: 'place',
        range: 'place',
        docIds: [],
      },
    ]);
    // The selection is spent, so a second click cannot re-apply it. Waiting is
    // part of the assertion: confirming swaps the bar into its confirm state and
    // the apply button only comes back once the batch has resolved.
    await waitFor(() =>
      expect(screen.getByTestId('ontology-quality-apply-batch')).toBeTruthy(),
    );
    expect(
      (screen.getByTestId('ontology-quality-apply-batch') as HTMLButtonElement).disabled,
    ).toBe(true);
    // The reader is told the work started, not left with a batch that seems to
    // have done nothing.
    expect(
      await screen.findByText(/affected documents are re-compiling/i),
    ).toBeTruthy();
  });

  // The reported bug: a property the template DOES declare, observed with an
  // endpoint pair it does not. Offering "declare" here hands the reader an edit
  // the writer refuses ("the template already declares …"), which fails the whole
  // batch. The applicable edit is the widen, applied to the side left out.
  it('offers a widen, not a declare, when only the endpoint pair is undeclared', () => {
    const onApplyFixes = jest.fn().mockResolvedValue(undefined);
    const graph = graphWith(baseQuality({ undeclared_properties: 0 }));
    graph.classes = [
      { type: 'work', declared: true, entities: 2 },
      { type: 'place', declared: true, entities: 1 },
      { type: 'season', declared: true, entities: 1 },
    ];
    graph.properties = [
      { type: 'part_of', source: 'work', target: 'place', relations: 0, declared: true },
      {
        type: 'part_of',
        source: 'work',
        target: 'season',
        relations: 4,
        declared: false,
        declared_name: true,
      },
    ];
    renderExpanded(graph, { onApplyFixes });

    // No declaration offered: the name is already declared.
    expect(screen.queryByTestId('ontology-quality-select-declare|part_of')).toBeNull();
    expect(screen.getByText(/Add “season” to part_of’s range/)).toBeTruthy();
    expect(screen.getByText(/declared: place/)).toBeTruthy();

    fireEvent.click(
      screen.getByTestId('ontology-quality-select-widenpair|part_of|range|season'),
    );
    fireEvent.click(screen.getByTestId('ontology-quality-apply-batch'));
    expect(onApplyFixes).toHaveBeenCalledWith([
      {
        op: 'widen',
        property: 'part_of',
        side: 'range',
        class: 'season',
        docIds: [],
      },
    ]);
  });

  // A zero only means "clean" when something was measured. A compile that typed
  // entities but produced no assertion at all shows three zeros that are an empty
  // set — reading them as a pass hides a failed extraction.
  it('says the zeros are an empty set when no assertion was compiled', () => {
    const graph = graphWith(baseQuality());
    graph.classes = [
      { type: 'artifact', declared: true, entities: 82 },
      { type: 'organization', declared: true, entities: 16 },
    ];
    graph.properties = [
      { type: 'created_by', source: 'artifact', target: 'organization', relations: 0, declared: true },
    ];
    renderExpanded(graph);

    const notice = screen.getByTestId('ontology-quality-nothing-measured');
    expect(notice.textContent).toContain('98');
    expect(notice.textContent).toContain('empty set');
  });

  // With assertions in scope the same zeros mean what they say, so the notice
  // must not appear and turn a clean compile into a warning.
  it('does not call the zeros empty when assertions were compiled', () => {
    const graph = graphWith(baseQuality());
    graph.properties = [
      { type: 'created_by', source: 'artifact', target: 'organization', relations: 7, declared: true },
    ];
    renderExpanded(graph);

    expect(screen.queryByTestId('ontology-quality-nothing-measured')).toBeNull();
  });

  // The reported bug: a property exercised on one of its declared pairs is not
  // "never exercised". `participant_in` carried 23 assertions and was still
  // listed, because fifteen of its sixteen declared pairs happened to be empty.
  it('lists a property only when none of its pairs was exercised', () => {
    const graph = graphWith(
      baseQuality({ properties_without_assertions: 15 }),
    );
    graph.properties = [
      { type: 'participant_in', source: 'person', target: 'event', relations: 23, declared: true },
      { type: 'participant_in', source: 'work', target: 'work', relations: 0, declared: true },
      { type: 'led_by', source: 'organization', target: 'person', relations: 0, declared: true },
    ];
    renderExpanded(graph);

    expect(screen.getByText('led_by')).toBeTruthy();
    expect(screen.queryByText('participant_in')).toBeNull();
    // Pairs are coverage, and they are reported as coverage.
    expect(screen.getByText(/1 of 3 declared endpoint pairs/)).toBeTruthy();
  });

  // The apply control has to sit above the lists, right under the line that tells
  // the reader to use it: buried below five sections — past "never exercised" —
  // it reads as if re-compiling the document is what applies the selection, and
  // then nothing is applied at all.
  it('puts the apply control before the lists it applies', () => {
    const onApplyFixes = jest.fn().mockResolvedValue(undefined);
    renderExpanded(graphWith(baseQuality({ dropped_relations: 1 })), {
      onApplyFixes,
    });

    const bar = screen.getByTestId('ontology-quality-apply-batch');
    const firstRow = screen.getByTestId('ontology-quality-dropped');
    expect(
      bar.compareDocumentPosition(firstRow) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  // A selection that has not been applied must be visible from outside the
  // detail, otherwise selecting and re-compiling from the page looks like a
  // no-op that the reader keeps repeating.
  it('marks an unapplied selection on the summary line', () => {
    const onApplyFixes = jest.fn().mockResolvedValue(undefined);
    renderExpanded(graphWith(baseQuality({ undeclared_properties: 1 })), {
      onApplyFixes,
    });

    expect(screen.queryByTestId('ontology-quality-pending')).toBeNull();
    fireEvent.click(screen.getByTestId('ontology-quality-select-declare|invented'));
    expect(screen.getByTestId('ontology-quality-pending').textContent).toContain(
      '1',
    );
    fireEvent.click(screen.getByTestId('ontology-quality-clear-selection'));
    expect(screen.queryByTestId('ontology-quality-pending')).toBeNull();
  });

  // The other half of the same rule: widening with a class the template does not
  // declare is refused too, so the gap is shown without a button instead of
  // becoming another refused edit.
  it('shows an undeclared endpoint without an edit when the class is undeclared too', () => {
    const onApplyFixes = jest.fn().mockResolvedValue(undefined);
    const graph = graphWith(baseQuality({ undeclared_properties: 0 }));
    graph.classes = [
      { type: 'work', declared: true, entities: 2 },
      { type: 'place', declared: true, entities: 1 },
    ];
    graph.properties = [
      { type: 'part_of', source: 'work', target: 'place', relations: 0, declared: true },
      {
        type: 'part_of',
        source: 'work',
        target: 'ghost',
        relations: 2,
        declared: false,
        declared_name: true,
      },
    ];
    renderExpanded(graph, { onApplyFixes });

    expect(screen.getByText(/ghost is not declared either/)).toBeTruthy();
    expect(
      screen.queryByTestId('ontology-quality-select-widenpair|part_of|range|ghost'),
    ).toBeNull();
  });

  // Deselecting is part of selecting: a reader who marked a line by mistake must
  // be able to take it out of the batch before compiling.
  it('lets the reader drop a line out of the batch', () => {
    const onApplyFixes = jest.fn().mockResolvedValue(undefined);
    renderExpanded(graphWith(baseQuality({ undeclared_properties: 1 })), {
      onApplyFixes,
    });

    const select = screen.getByTestId('ontology-quality-select-declare|invented') as HTMLInputElement;
    fireEvent.click(select);
    expect(select.checked).toBe(true);
    fireEvent.click(screen.getByTestId('ontology-quality-clear-selection'));
    expect(
      (screen.getByTestId('ontology-quality-apply-batch') as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  // Without a host that can write, the ledger stays advice: no control that
  // cannot do anything.
  it('shows no selection controls when the host cannot apply edits', () => {
    renderExpanded(
      graphWith(
        baseQuality({
          undeclared_properties: 1,
          dropped_relations: 1,
          dropped_samples: [{ property: 'part_of', from_type: 'event', to_type: 'work' }],
        }),
      ),
    );
    expect(screen.queryByTestId('ontology-quality-apply-batch')).toBeNull();
    expect(
      screen.queryByTestId('ontology-quality-select-declare|invented'),
    ).toBeNull();
  });

  // The detail is a bounded surface of its own, so however long the lists get
  // they scroll inside it instead of growing the page header.
  it('bounds the details so they scroll instead of growing', () => {
    renderExpanded(
      graphWith(baseQuality({ undeclared_properties: 1 })),
    );
    const details = screen.getByTestId('ontology-quality-details');
    expect(details.className).toContain('max-h-[32vh]');
    expect(details.className).toContain('overflow-y-auto');

    const body = screen
      .getByTestId('ontology-quality-undeclared')
      .querySelector('div.overflow-y-auto');
    expect(body?.className).toContain('max-h-24');
  });

  // A rejection used to be silent. Listing the subject plus the compiler's own
  // sentence is what makes a rejected assertion actionable: the reader sees which
  // declaration to widen.
  it('lists rejected assertions with the reason, the endpoints and the document', () => {
    renderExpanded(
      graphWith(
        baseQuality({
          dropped_relations: 1,
          dropped_samples: [
            {
              property: 'born_in',
              from: 'Ada',
              to: 'Paris',
              from_type: 'place',
              to_type: 'person',
              reason:
                'dropped born_in: the template declares domain person, the assertion used "place"',
              doc_id: 'doc-1',
            },
          ],
        }),
      ),
    );

    const row = screen.getByTestId('ontology-quality-dropped');
    expect(row.textContent).toContain('1');
    // The action, not the symptom: the class to add, the property, the side and
    // what is declared today.
    expect(row.textContent).toContain('Add “place” to born_in’s domain');
    expect(row.textContent).toContain('declared: person');
    expect(row.textContent).toContain('Ada');
    // The compiler's own sentence rides along as the tooltip.
    const action = screen.getByTestId('ontology-quality-action-born_in|domain|place');
    expect(action.getAttribute('title')).toContain(
      'the template declares domain person',
    );
  });

  // Vocabulary drift and the declarations the scope never exercised are the two
  // lists the reader acts on by editing the template, so they name their
  // subjects rather than only counting them.
  it('names the undeclared properties and the never-exercised declarations', () => {
    renderExpanded(
      graphWith(
        baseQuality({
          undeclared_properties: 1,
          classes_without_instances: 1,
          properties_without_assertions: 1,
        }),
      ),
    );

    expect(
      screen.getByTestId('ontology-quality-undeclared').textContent,
    ).toContain('invented');
    const coverage = screen.getByTestId('ontology-quality-coverage');
    expect(coverage.textContent).toContain('person');
    expect(coverage.textContent).toContain('born_in');
    // place has instances and place->place has relations, so neither belongs here.
    expect(coverage.textContent).not.toContain('place');
  });

  // A declared property reaches the panel once per (source, target) pair, so the
  // same name arrives several times. Listing it once per pair reads as several
  // problems, and the count then disagrees with the names below it — both were
  // visible on a real template (`part_of` listed five times).
  it('names a never-exercised property once, not once per declaration pair', () => {
    const graph = graphWith(
      baseQuality({
        classes_without_instances: 0,
        properties_without_assertions: 3,
      }),
    );
    graph.properties = [
      { type: 'part_of', source: 'event', target: 'work', relations: 0, declared: true },
      { type: 'part_of', source: 'season', target: 'work', relations: 0, declared: true },
      { type: 'part_of', source: 'episode', target: 'work', relations: 0, declared: true },
    ];
    renderExpanded(graph);

    const coverage = screen.getByTestId('ontology-quality-coverage');
    expect(coverage.textContent?.match(/part_of/g)?.length).toBe(1);
    // The header agrees with what is listed (the fixture's `person` class is
    // uncovered too), not with the backend's three-per-pair total.
    expect(coverage.textContent).toContain('2');
  });

  // When the backend caps the sample list, the panel must say so: a truncated
  // list that looks complete is how a reader concludes the rest is fine.
  it('says when only a page of the rejections is listed', () => {
    renderExpanded(
      graphWith(
        baseQuality({
          dropped_relations: 120,
          samples_truncated: true,
          dropped_samples: [{ property: 'born_in' }],
        }),
      ),
    );

    const row = screen.getByTestId('ontology-quality-dropped');
    expect(row.textContent).toContain('120');
    expect(row.textContent).toContain('the count above is complete');
  });

  // The fix lives in the template, so when the host can drive the canvas the
  // subject is a button that takes the reader to the declaring class.
  it('focuses the declaring class when the host provides that', () => {
    const onFocusClass = jest.fn();
    renderExpanded(graphWith(baseQuality({ undeclared_properties: 1 })), {
      onFocusClass,
    });

    fireEvent.click(
      screen.getByTestId('ontology-quality-undeclared').querySelector('button')!,
    );
    expect(onFocusClass).toHaveBeenCalledWith('place');
  });
});
