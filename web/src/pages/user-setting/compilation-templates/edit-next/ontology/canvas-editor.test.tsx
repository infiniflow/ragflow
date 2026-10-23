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
import { useForm } from 'react-hook-form';

import { OntologyCanvasEditor } from './canvas-editor';
import { OntologyClassKeys, OntologyPropertyKeys } from './model';

// The canvas measures the box it was given with a ResizeObserver, which jsdom does
// not provide. The mock reports a fixed box: the panel's resize limits are computed
// from the canvas' width, so a zero-width reading would make every ceiling
// meaningless and leave the drag untested.
class ResizeObserverMock {
  constructor(private callback?: (entries: unknown[]) => void) {}

  observe() {
    this.callback?.([{ contentRect: { width: 900, height: 600 } }]);
  }

  unobserve() {}

  disconnect() {}
}
(globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverMock;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, options?: any) => options?.defaultValue ?? _key,
  }),
}));

// The dropdown is cmdk-based and needs a browser layout; these tests are about
// what the canvas WRITES, so it is replaced with a native select.
jest.mock('@/components/originui/select-with-search', () => ({
  SelectWithSearch: ({ value, options, onChange }: any) => (
    <select
      data-testid="mock-select"
      value={value}
      onChange={(event) => onChange?.(event.target.value)}
    >
      {(options ?? []).map((option: any) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
}));

// `any` throughout: this file contains jest.mock, so it goes through
// esbuild-jest's babel pass, which cannot strip imported types used as
// annotations (see use-memory-request.test.tsx).
const defaultValues: any = {
  name: 'group',
  templates: [
    {
      name: 'Ontology',
      kind: 'ontology',
      config: {
        kind: 'ontology',
        base_uri: 'http://example.org/ontology#',
        entity: {
          // A section key the canvas does not model: it must survive every edit.
          output_fields: [{ name: 'name', shape: 'text', required: true }],
          fields: [
            {
              type: 'agent',
              label: 'Agent',
              description: 'Anything that acts.',
              parent: '',
              rule: '',
            },
            {
              type: 'person',
              label: 'Person',
              description: 'A human.',
              parent: 'agent',
              rule: '',
            },
          ],
        },
        relation: {
          fields: [
            {
              type: 'birth_date',
              kind: 'datatype',
              domain: 'person',
              range: '',
              datatype: 'date',
              label: 'birth date',
              description: 'When someone was born.',
              rule: '',
            },
          ],
        },
      },
    },
  ],
};

const renderEditor = () => {
  const form: any = { current: null };
  const Harness = () => {
    const methods = useForm<any>({ defaultValues });
    form.current = methods;
    return <OntologyCanvasEditor form={methods} templateIndex={0} />;
  };
  render(<Harness />);
  return form;
};

// A fixture of its own for the geometry cases: the default one declares a single
// datatype property, which draws no edge at all.
const renderEditorWith = (mutate: (values: any) => void) => {
  const values: any = JSON.parse(JSON.stringify(defaultValues));
  mutate(values);
  const form: any = { current: null };
  const Harness = () => {
    const methods = useForm<any>({ defaultValues: values });
    form.current = methods;
    return <OntologyCanvasEditor form={methods} templateIndex={0} />;
  };
  render(<Harness />);
  return form;
};

const pointsOf = (path: string) =>
  [...path.matchAll(/(-?[\d.]+),(-?[\d.]+)/g)].map((match) => ({
    x: Number(match[1]),
    y: Number(match[2]),
  }));

/**
 * The requirement, stated once: every segment is horizontal or vertical, so every
 * bend is a right angle.
 */
const expectOrthogonal = (points: { x: number; y: number }[]) => {
  points.slice(1).forEach((point, at) => {
    const previous = points[at];
    expect(point.x === previous.x || point.y === previous.y).toBe(true);
  });
};

const fieldsOf = (form: any, section: 'entity' | 'relation') =>
  form.current.getValues(`templates.0.config.${section}`);

describe('OntologyCanvasEditor', () => {
  it('draws the declared classes and their object properties', () => {
    renderEditor();
    expect(screen.getByTestId('ontology-node-agent')).toBeTruthy();
    expect(screen.getByTestId('ontology-node-person')).toBeTruthy();
    // Inheritance is drawn as its own edge, like the read-only model graph.
    expect(
      screen.getByTestId('ontology-inheritance-agent-person'),
    ).toBeTruthy();
  });

  // The one thing the canvas must never get wrong: what it writes is the
  // template's own field shape, in the template's own key order.
  it('writes an added class in the template field shape', () => {
    const form = renderEditor();
    const before = fieldsOf(form, 'entity').fields.length;
    fireEvent.click(screen.getByText('+ Class'));

    const fields = fieldsOf(form, 'entity').fields;
    expect(fields).toHaveLength(before + 1);
    expect(Object.keys(fields[before])).toEqual([...OntologyClassKeys]);
  });

  // An attribute is created FROM a class, so its domain is never empty. A
  // property with no domain draws nothing on the canvas and belonged to nobody —
  // which is exactly the confusion the two toolbar buttons caused.
  it('creates an attribute anchored to the selected class', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByTestId('ontology-node-agent'));
    fireEvent.click(screen.getByTestId('ontology-add-own-attribute'));

    const fields = fieldsOf(form, 'relation').fields;
    const added = fields[fields.length - 1];
    expect(Object.keys(added)).toEqual([...OntologyPropertyKeys]);
    expect(added.kind).toBe('datatype');
    expect(added.datatype).toBe('string');
    expect(added.domain).toBe('agent');
  });

  // "Which entity does this property belong to" has to be answerable on the class
  // itself, not by opening each property in turn.
  it('lists the properties of the selected class', () => {
    renderEditor();
    fireEvent.click(screen.getByTestId('ontology-node-person'));

    // The list shows the same string the node prints — the label — so the panel
    // and the canvas never disagree about what a property is called.
    const listed = screen.getByTestId('ontology-own-properties');
    expect(listed.textContent).toContain('birth date');

    fireEvent.click(screen.getByTestId('ontology-node-agent'));
    expect(screen.getByTestId('ontology-own-properties').textContent).toContain(
      'None yet.',
    );
  });

  // The panel says out loud when a property has no domain, because such a property
  // is invisible on the canvas — silence is what read as "broken".
  it('says a property with no domain draws nothing', () => {
    renderEditorWith((values) => {
      values.templates[0].config.relation.fields = [
        {
          type: 'orphan_relation',
          kind: 'object',
          domain: '',
          range: '',
          datatype: '',
        },
      ];
    });
    // It draws no edge, so the issue list is the only way in — which is why the
    // issues are clickable.
    fireEvent.click(screen.getByTestId('ontology-issue-missing_domain'));

    expect(screen.getByTestId('ontology-property-owner').textContent).toContain(
      'no class yet',
    );
  });

  // A section is not only `{description, fields}`: `entity.output_fields` is part
  // of the builtin ontology template, and the canvas rewrites whole sections.
  it('carries through keys the canvas does not own', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByText('+ Class'));

    const entity = fieldsOf(form, 'entity');
    expect(entity.output_fields).toEqual([
      { name: 'name', shape: 'text', required: true },
    ]);
  });

  // Two clicks on the canvas, one relationship: the handle is how an object
  // property is created visually, and the endpoints have to land in the
  // template's own keys with the template's own order.
  it('creates an object property from a link handle and a target class', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByTestId('ontology-link-handle-agent'));
    fireEvent.click(screen.getByTestId('ontology-node-person'));

    const fields = fieldsOf(form, 'relation').fields;
    const added = fields[fields.length - 1];
    expect(added.kind).toBe('object');
    expect(added.domain).toBe('agent');
    expect(added.range).toBe('person');
    expect(Object.keys(added)).toEqual([...OntologyPropertyKeys]);
  });

  // The handle used to only change a faint circle's opacity, so clicking it read
  // as broken. Two things make it legible: a banner that says what the next click
  // means, and Escape to back out.
  it('announces the linking mode and cancels it on Escape', () => {
    renderEditor();
    expect(screen.queryByTestId('ontology-linking-banner')).toBeNull();

    fireEvent.click(screen.getByTestId('ontology-link-handle-agent'));
    expect(screen.getByTestId('ontology-linking-banner')).toBeTruthy();

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByTestId('ontology-linking-banner')).toBeNull();
  });

  // Clicking empty canvas while linking is the reading a "+" invites: a new class
  // appears where the reader clicked, already joined to the source.
  it('adds a class joined to the source when empty canvas is clicked', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByTestId('ontology-link-handle-agent'));
    fireEvent.click(screen.getByRole('img'));

    const classes = fieldsOf(form, 'entity').fields;
    const properties = fieldsOf(form, 'relation').fields;
    const created = classes[classes.length - 1];
    const added = properties[properties.length - 1];

    expect(classes).toHaveLength(3);
    expect(Object.keys(created)).toEqual([...OntologyClassKeys]);
    expect(added.kind).toBe('object');
    expect(added.domain).toBe('agent');
    expect(added.range).toBe(created.type);
    // Creating the class ends the linking mode: the next click has to mean
    // whatever the canvas normally means, not "connect to agent again".
    expect(screen.queryByTestId('ontology-linking-banner')).toBeNull();
  });

  // Several properties between the same pair are drawn as ONE edge. N curves with
  // N labels between two boxes cannot be read: a quadratic Bézier's midpoint moves
  // only half as far as its control point, so even a 40px fan leaves 20px between
  // two 11px labels. The count goes on the edge and the panel lists the contents.
  it('bundles the properties between a pair into one edge', () => {
    renderEditorWith((values) => {
      const relation = values.templates[0].config.relation;
      relation.fields = [
        {
          ...relation.fields[0],
          type: 'employed_by',
          kind: 'object',
          domain: 'person',
          range: 'agent',
          datatype: '',
          label: 'employed by',
        },
        {
          ...relation.fields[0],
          type: 'manages',
          kind: 'object',
          domain: 'agent',
          range: 'person',
          datatype: '',
          label: 'manages',
        },
      ];
    });

    // One element, not two — the whole point.
    const bundle = screen.getByTestId('ontology-edge-bundle-agent|person');
    expect(bundle.textContent).toContain('×2');
    // The fixture declares one property in each direction, so the bundle must
    // mark BOTH ends: a single arrowhead would misstate half of what it holds.
    expect(bundle.querySelector('path')?.getAttribute('marker-start')).toBe(
      'url(#ontology-arrow)',
    );
    expect(screen.queryByTestId('ontology-edge-employed_by')).toBeNull();
    expect(screen.queryByTestId('ontology-edge-manages')).toBeNull();

    // The panel answers "what is in it", which is what the count hides.
    fireEvent.click(bundle);
    // Matching a substring: the `t` mock in this file returns the raw
    // defaultValue, so it does not interpolate `{{count}}`.
    expect(screen.getByText(/properties share this edge/)).toBeTruthy();
    expect(screen.getByText('employed by')).toBeTruthy();
    expect(screen.getByText('manages')).toBeTruthy();
  });

  // A lone property keeps its own name, so the common case never regresses into
  // "1 property" and stays clickable straight into its editor.
  it('keeps a lone property edge named after the property', () => {
    renderEditorWith((values) => {
      values.templates[0].config.relation.fields = [
        {
          type: 'born_in',
          kind: 'object',
          domain: 'person',
          range: 'agent',
          datatype: '',
        },
      ];
    });

    const edge = screen.getByTestId('ontology-edge-born_in');
    fireEvent.click(edge);
    expect(screen.getByTestId('ontology-property-owner').textContent).toContain(
      'person → agent',
    );
  });

  // The inheritance line is a connector as well and gets the same right angles: a
  // dashed diagonal next to orthogonally routed properties reads as a different
  // kind of edge, which it is not.
  it('routes the inheritance line orthogonally too', () => {
    renderEditor();
    // The test id sits on the path itself here (an edge puts it on its group).
    const attributes = screen
      .getByTestId('ontology-inheritance-agent-person')
      .getAttribute('d');
    expect(attributes).toBeTruthy();

    const points = pointsOf(attributes!);
    expectOrthogonal(points);
    // agent (y 40..92) and person (y 40..108, it carries an attribute) are not
    // aligned, so the route turns rather than running straight.
    expect(points.length).toBeGreaterThan(2);
  });

  // The arrow has to POINT AT the class, which means both ends sit on a border. A
  // connector drawn between centres runs under the box and buries the arrowhead
  // inside it.
  it('anchors both ends of a connector on the box borders', () => {
    renderEditorWith((values) => {
      values.templates[0].config.relation.fields = [
        {
          type: 'born_in',
          kind: 'object',
          domain: 'person',
          range: 'agent',
          datatype: '',
        },
      ];
    });

    const path = screen
      .getByTestId('ontology-edge-born_in')
      .querySelector('path')!
      .getAttribute('d')!;
    const points = pointsOf(path);
    expectOrthogonal(points);

    // agent is the first cell (x 40..240), person the second (x 300..500), so the
    // line has to start on 300 and end on 240 — not on the 400/140 centres.
    expect(points).toHaveLength(2);
    expect(points[0].x).toBeCloseTo(300, 0);
    expect(points[1].x).toBeCloseTo(240, 0);

    // And the label clears the link handle: agent's handle is a circle on its right
    // edge (x 240..260, y 56..76), and the label sits level with the line, so only
    // its height decides. It has to stay above the handle's band.
    const label = screen
      .getByTestId('ontology-edge-born_in')
      .querySelector('text')!;
    expect(Number(label.getAttribute('y'))).toBeLessThan(56);
  });

  // A connector must not run over a class that stands between its ends. The detour
  // is a polyline, because a bend deviates by its offset exactly while a curve only
  // deviates half as far.
  it('routes a connector around a class that stands in the way', () => {
    renderEditorWith((values) => {
      values.templates[0].config.entity.fields = [
        { type: 'agent', label: 'Agent', parent: '', description: 'a', rule: '' },
        { type: 'person', label: 'Person', parent: '', description: 'p', rule: '' },
        { type: 'place', label: 'Place', parent: '', description: 'l', rule: '' },
      ];
      values.templates[0].config.relation.fields = [
        {
          type: 'far',
          kind: 'object',
          domain: 'agent',
          range: 'place',
          datatype: '',
          label: 'far',
        },
      ];
    });

    const path = screen
      .getByTestId('ontology-edge-far')
      .querySelector('path')!
      .getAttribute('d')!;
    const points = pointsOf(path);
    expectOrthogonal(points);
    // Out of agent's right edge (240), across in the lane below person, into
    // place's left edge (560).
    expect(points[0]).toEqual({ x: 240, y: 66 });
    expect(points[points.length - 1]).toEqual({ x: 560, y: 66 });
    // person sits at y 40..92 and is what a straight line would cross, so the run
    // has to leave that band.
    expect(Math.max(...points.map((point) => point.y))).toBeGreaterThan(92);
  });

  // One detour has to clear ALL of them: routing around only the nearest obstacle
  // leaves the next one crossing the line — which is what a row (or column) of
  // classes between two endpoints produces.
  it('clears every class between the ends, not just the first', () => {
    renderEditorWith((values) => {
      values.templates[0].config.entity.fields = [
        { type: 'first', label: 'First', parent: '', description: 'a', rule: '' },
        { type: 'second', label: 'Second', parent: '', description: 'b', rule: '' },
        { type: 'third', label: 'Third', parent: '', description: 'c', rule: '' },
        { type: 'fourth', label: 'Fourth', parent: '', description: 'd', rule: '' },
        { type: 'fifth', label: 'Fifth', parent: '', description: 'e', rule: '' },
      ];
      values.templates[0].config.relation.fields = [
        {
          type: 'spans',
          kind: 'object',
          domain: 'first',
          range: 'fourth',
          datatype: '',
          label: 'spans',
        },
      ];
    });

    const path = screen
      .getByTestId('ontology-edge-spans')
      .querySelector('path')!
      .getAttribute('d')!;
    const points = pointsOf(path);
    expectOrthogonal(points);

    // Four classes to a row: first (40) .. second (300) .. third (560) .. fourth
    // (820), each 200 wide and 52 tall. The run goes out of first's right edge,
    // drops into the empty lane under the row, and comes back into fourth.
    expect(points[0]).toEqual({ x: 240, y: 66 });
    expect(points[points.length - 1]).toEqual({ x: 820, y: 66 });

    // The lane has to straddle EVERY blocker — a turn before `second` (300) and
    // another after `third` (760). Routing around only the nearest would leave the
    // second one crossing the line.
    const lane = points.filter((point) => point.y > 92);
    expect(lane.some((point) => point.x < 300)).toBe(true);
    expect(lane.some((point) => point.x > 760)).toBe(true);
  });

  // domain == range used to draw a zero-length line, so a reflexive property was
  // invisible. The loop stands ABOVE the node: the right edge belongs to the link
  // handle, and a loop plus a label plus a control do not fit in the 60px between
  // two columns — the label was landing on the handle.
  it('draws a reflexive property as a loop clear of the link handle', () => {
    renderEditorWith((values) => {
      values.templates[0].config.relation.fields = [
        {
          type: 'knows',
          kind: 'object',
          domain: 'person',
          range: 'person',
          datatype: '',
        },
      ];
    });

    const group = screen.getByTestId('ontology-edge-knows');
    const points = pointsOf(group.querySelector('path')!.getAttribute('d')!);
    expectOrthogonal(points);

    // person is the second cell: x 300..500, y 40..92 (no attributes), so the loop
    // starts and ends on its top border.
    expect(points).toHaveLength(4);
    expect(points[0]).toEqual({ x: 386, y: 40 });
    expect(points[3]).toEqual({ x: 414, y: 40 });
    // Upwards, and never as far right as the handle (which sits at x 500..520,
    // y 56..76, outside the right edge).
    expect(Math.min(...points.map((point) => point.y))).toBeLessThan(40);
    expect(Math.max(...points.map((point) => point.x))).toBeLessThan(500);

    const label = group.querySelector('text')!;
    expect(label.getAttribute('x')).toBe('422');
    // Above the box, so nowhere near the handle's band either.
    expect(Number(label.getAttribute('y'))).toBeLessThan(40);
  });

  // The panel is a form, not a legend: it starts 50% wider than the old sidebar and
  // the reader can trade canvas for it. The divider is a control, so it also works
  // from the keyboard and resets on a double click.
  it('resizes the panel from the divider', () => {
    renderEditor();
    const panel = screen.getByTestId('ontology-detail-panel');
    expect(panel.style.width).toBe('432px');

    const divider = screen.getByTestId('ontology-panel-resizer');
    // Mouse events, not pointer events: jsdom has no `PointerEvent` constructor, so
    // a pointer-dispatched event loses `clientX` and nothing can be driven. The
    // divider listens to both.
    fireEvent.mouseDown(divider, { clientX: 1000 });
    fireEvent.mouseMove(divider, { clientX: 940 });
    fireEvent.mouseUp(divider);
    // Dragging the divider left widens the panel.
    expect(panel.style.width).toBe('492px');

    fireEvent.keyDown(divider, { key: 'ArrowRight' });
    expect(panel.style.width).toBe('476px');
    fireEvent.keyDown(divider, { key: 'ArrowLeft' });
    expect(panel.style.width).toBe('492px');

    fireEvent.doubleClick(divider);
    expect(panel.style.width).toBe('432px');
  });

  // Neither end can be squeezed away: the panel keeps its minimum, and the canvas
  // keeps `MinCanvasWidth`. The mock reports a 900px canvas, so the ceiling is
  // 432 + 900 - 280 = 1052.
  it('keeps both the panel and the canvas usable', () => {
    renderEditor();
    const panel = screen.getByTestId('ontology-detail-panel');
    const divider = screen.getByTestId('ontology-panel-resizer');

    fireEvent.mouseDown(divider, { clientX: 0 });
    fireEvent.mouseMove(divider, { clientX: 4000 });
    fireEvent.mouseUp(divider);
    expect(panel.style.width).toBe('260px');

    // Back the other way: the ceiling is measured from the state at the START of
    // the drag, so a 260px panel beside a 900px canvas may grow to 260 + 900 - 280
    // = 880 before the canvas would drop under its minimum.
    fireEvent.mouseDown(divider, { clientX: 4000 });
    fireEvent.mouseMove(divider, { clientX: 0 });
    fireEvent.mouseUp(divider);
    expect(panel.style.width).toBe('880px');
  });

  it('edits the selected class through the panel', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByTestId('ontology-node-person'));
    fireEvent.change(screen.getByTestId('ontology-class-description'), {
      target: { value: 'A person.' },
    });

    const person = fieldsOf(form, 'entity').fields.find(
      (field: any) => field.type === 'person',
    );
    expect(person.description).toBe('A person.');
  });

  // Renaming a class has to rewrite every reference to it, or the template
  // silently loses the edges that pointed at the old name.
  it('rewrites references when a class is renamed', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByTestId('ontology-node-person'));
    fireEvent.change(screen.getByTestId('ontology-class-type'), {
      target: { value: 'human' },
    });

    const sections = fieldsOf(form, 'relation');
    expect(sections.fields[0].domain).toBe('human');
  });

  it('reports what the backend would reject', () => {
    const form = renderEditor();
    fireEvent.click(screen.getByTestId('ontology-node-person'));
    fireEvent.change(screen.getByTestId('ontology-class-description'), {
      target: { value: '' },
    });

    // Listed twice on purpose: the summary lists every issue, the side panel
    // repeats the ones about what is selected.
    expect(
      screen.getAllByText('Class person needs a description.').length,
    ).toBeGreaterThan(0);
    void form;
  });
});
