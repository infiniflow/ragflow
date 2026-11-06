import type { IOntologyGraph } from '@/interfaces/database/document-structure';
import {
  buildOntologyGraphData,
  computeOntologyLayout,
} from './graph-data';

const graphOf = (overrides: Partial<IOntologyGraph> = {}): IOntologyGraph => ({
  classes: [],
  properties: [],
  total_entities: 0,
  total_relations: 0,
  counts_truncated: false,
  unattributed_relations: 0,
  ...overrides,
});

describe('computeOntologyLayout', () => {
  // The shipped ontology reduced to its SHAPE: a two-level hierarchy whose root
  // holds no property edge at all, and two classes that belong to no hierarchy —
  // `artifact` has no edge of any kind, which is what a force layout turned into
  // "this class sits far away from everything".
  const graph = graphOf({
    classes: [
      { type: 'agent', entities: 0, declared: true },
      { type: 'person', entities: 52, declared: true, parent: ['agent'] },
      { type: 'organization', entities: 15, declared: true, parent: ['agent'] },
      { type: 'place', entities: 13, declared: true },
      { type: 'artifact', entities: 3, declared: true },
    ],
    properties: [
      {
        type: 'born_in',
        source: 'person',
        target: 'place',
        relations: 3,
        declared: true,
      },
      {
        type: 'member_of',
        source: 'person',
        target: 'organization',
        relations: 2,
        declared: true,
      },
      {
        type: 'headquartered_in',
        source: 'organization',
        target: 'place',
        relations: 1,
        declared: true,
      },
    ],
    inheritance: [
      { source: 'agent', target: 'person' },
      { source: 'agent', target: 'organization' },
    ],
  });

  const boxOf = (data: ReturnType<typeof buildOntologyGraphData>) => {
    const xs = data.nodes.map((node) => node.x ?? 0);
    const ys = data.nodes.map((node) => node.y ?? 0);
    return {
      width: Math.max(...xs) - Math.min(...xs),
      height: Math.max(...ys) - Math.min(...ys),
    };
  };

  // The regression this guards is the ring layout: a circle is inscribed by the
  // SHORTER side of the canvas, so on a wide panel the graph filled the middle
  // and left both sides empty. Filling the rectangle is the whole point of the
  // relaxed layout, so it is asserted on a wide, a square-ish and a small canvas.
  it('uses the canvas instead of leaving most of it empty', () => {
    for (const [width, height] of [
      [600, 400],
      [1200, 800],
      [900, 420],
    ]) {
      const data = buildOntologyGraphData(graph, { width, height });
      const box = boxOf(data);
      expect(box.width).toBeGreaterThan(width * 0.6);
      expect(box.height).toBeGreaterThan(height * 0.6);
    }
  });

  it('never lets two class discs touch, at any canvas size', () => {
    for (const [width, height] of [
      [600, 400],
      [320, 240],
      [1200, 800],
    ]) {
      const data = buildOntologyGraphData(graph, { width, height });
      const layout = computeOntologyLayout(data.nodes, data.links, width, height);
      const placed = data.nodes
        .map((node) => ({ node, point: layout.get(node.id) }))
        .filter((item) => item.point);
      expect(placed).toHaveLength(data.nodes.length);
      for (let i = 0; i < placed.length; i += 1) {
        for (let j = i + 1; j < placed.length; j += 1) {
          const a = placed[i];
          const b = placed[j];
          const gap = Math.hypot(
            a.point!.x - b.point!.x,
            a.point!.y - b.point!.y,
          );
          expect(gap).toBeGreaterThan(a.node.radius + b.node.radius);
        }
      }
    }
  });

  it('is deterministic, so the picture does not depend on the physics', () => {
    const first = buildOntologyGraphData(graph, { width: 600, height: 400 });
    const second = buildOntologyGraphData(graph, { width: 600, height: 400 });
    expect(first.nodes.map((node) => [node.id, node.x, node.y])).toEqual(
      second.nodes.map((node) => [node.id, node.x, node.y]),
    );
    // Every node is PLACED, and none of them is PINNED. The distinction is what
    // keeps the view readable: the layout decides where a class starts, but
    // `fx`/`fy` would make it immovable — and dragging a class out of a crowd is
    // how a reader untangles a dense ontology. The canvas removes the forces, so
    // nothing moves the node before the reader does.
    expect(first.nodes.every((node) => node.x !== undefined)).toBe(true);
    expect(first.nodes.every((node) => node.fx === undefined)).toBe(true);
  });

  // A class with no edge has nothing holding it, so repulsion used to walk it out
  // to the rim — and since the layout is then FITTED to the canvas, that one
  // class also defined the bounding box: it ended up on the edge of the picture
  // while everything related was squeezed into the middle. Measured on the
  // shipped shape, it sat 538px from the cluster centre while the next furthest
  // class was 317.
  it('keeps a class that has no edge among the others', () => {
    const data = buildOntologyGraphData(graph, { width: 900, height: 600 });
    const centre = {
      x:
        data.nodes.reduce((sum, node) => sum + (node.x ?? 0), 0) /
        data.nodes.length,
      y:
        data.nodes.reduce((sum, node) => sum + (node.y ?? 0), 0) /
        data.nodes.length,
    };
    const away = (id: string) => {
      const node = data.nodes.find((item) => item.id === id);
      return Math.hypot((node?.x ?? 0) - centre.x, (node?.y ?? 0) - centre.y);
    };
    // `artifact` is the class the fixture gives no property edge at all.
    const others = data.nodes
      .filter((node) => node.id !== 'artifact')
      .map((node) => away(node.id));
    expect(away('artifact')).toBeLessThan(Math.max(...others));
  });

  // A template with no inheritance at all has no skeleton to lay out, and must
  // still spread: the relaxed layout does not care where the edges come from.
  it('spreads a flat template that declares no inheritance', () => {
    const flat = graphOf({
      classes: ['a', 'b', 'c', 'd'].map((type) => ({
        type,
        entities: 1,
        declared: true,
      })),
      properties: [
        {
          type: 'rel',
          source: 'a',
          target: 'b',
          relations: 1,
          declared: true,
        },
      ],
    });
    const data = buildOntologyGraphData(flat, { width: 600, height: 400 });
    const box = boxOf(data);
    // A four-node graph with one edge is the lopsided case: the fitting cap
    // (which is what guarantees the discs cannot be squeezed into each other)
    // leaves a little space on the short axis.
    expect(box.width).toBeGreaterThan(600 * 0.6);
    expect(box.height).toBeGreaterThan(400 * 0.55);
  });
});

describe('buildOntologyGraphData', () => {
  it('prints the label but keeps the class name as the node id', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [
          { type: 'person', label: 'Person', entities: 3, declared: true },
        ],
      }),
    );

    // The name is what edge endpoints, filters and drill-down URLs are keyed by,
    // so it must survive even when a label is present.
    expect(data.nodes[0].id).toBe('person');
    expect(data.nodes[0].label).toBe('Person');
  });

  it('falls back to the name when the template declares no label', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [{ type: 'taxon', entities: 0, declared: true }],
      }),
    );

    expect(data.nodes[0].label).toBe('taxon');
  });

  it('carries the effective datatype attributes on the node', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [
          {
            type: 'person',
            entities: 2,
            declared: true,
            attributes: [
              { type: 'alias', datatype: 'list', assertions: 4 },
              {
                type: 'born_in',
                datatype: 'place',
                inherited_from: 'agent',
                assertions: 1,
              },
            ],
          },
        ],
      }),
    );

    expect(data.nodes[0].attributes).toHaveLength(2);
    // An inherited attribute reports its declarer, which is what lets the panel
    // separate own from inherited without re-walking the parent chain.
    expect(data.nodes[0].attributes[1].inherited_from).toBe('agent');
  });

  it('draws inheritance as its own edge kind, unlabelled and uncounted', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [
          { type: 'agent', entities: 0, declared: true },
          { type: 'person', label: 'Person', entities: 2, declared: true },
        ],
        inheritance: [{ source: 'agent', target: 'person' }],
      }),
    );

    expect(data.links).toHaveLength(1);
    const link = data.links[0];
    expect(link.kind).toBe('inheritance');
    expect(link.source).toBe('agent');
    expect(link.target).toBe('person');
    // rdfs:subClassOf has no vocabulary and no count: a label or a scaled width
    // would make it read as a property.
    expect(link.label).toBe('');
    expect(link.relations).toBe(0);
  });

  it('drops inheritance entirely when it is switched off', () => {
    const graph = graphOf({
      classes: [
        { type: 'agent', entities: 0, declared: true },
        { type: 'person', entities: 2, declared: true },
      ],
      inheritance: [{ source: 'agent', target: 'person' }],
    });

    expect(buildOntologyGraphData(graph, { showInheritance: false }).links).toHaveLength(0);
    expect(buildOntologyGraphData(graph, { showInheritance: true }).links).toHaveLength(1);
  });

  // An ontology declares several properties between the same two classes
  // (`born_in`, `died_in`, `citizen_of` are all person → place). Drawing one
  // curve each is what buried the classes under near-duplicate edges.
  it('bundles one class pair into a single labelled edge', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [
          { type: 'person', entities: 1, declared: true },
          { type: 'place', entities: 1, declared: true },
        ],
        properties: [
          {
            type: 'born_in',
            label: 'born in',
            source: 'person',
            target: 'place',
            relations: 5,
            declared: true,
          },
          {
            type: 'died_in',
            label: 'died in',
            source: 'person',
            target: 'place',
            relations: 3,
            declared: true,
          },
          {
            type: 'located_in',
            source: 'person',
            target: 'place',
            relations: 1,
            declared: true,
          },
        ],
      }),
    );

    expect(data.links).toHaveLength(1);
    const link = data.links[0];
    expect(link.kind).toBe('property');
    // Declaration order, so the label is stable run to run. The label falls back
    // to the property name when the template declares no label.
    expect(link.types).toEqual(['born_in', 'died_in', 'located_in']);
    expect(link.label).toBe('born in · died in · located_in');
    expect(link.relations).toBe(9);
    expect(link.declared).toBe(true);
    expect(link.undeclaredCount).toBe(0);
  });

  // Bundling is per ORDERED pair: direction is the only thing an object property
  // asserts, so the two directions must survive as two edges.
  it('keeps the two directions of a class pair apart', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [
          { type: 'person', entities: 1, declared: true },
          { type: 'organization', entities: 1, declared: true },
        ],
        properties: [
          {
            type: 'member_of',
            source: 'person',
            target: 'organization',
            relations: 2,
            declared: true,
          },
          {
            type: 'led_by',
            source: 'organization',
            target: 'person',
            relations: 4,
            declared: true,
          },
        ],
      }),
    );

    expect(data.links).toHaveLength(2);
    const [forward, backward] = data.links;
    expect(forward.types).toEqual(['member_of']);
    expect(backward.types).toEqual(['led_by']);
    // Opposite directions bow the other way, or one arrow hides the other.
    expect(Math.sign(forward.curvature)).toBe(-Math.sign(backward.curvature));
  });

  // A bundle is drift-free only when every property in it is declared — and the
  // undeclared filter runs BEFORE bundling, so a drifting property only drags its
  // bundle down while drift is being shown at all.
  it('marks a bundle undeclared when one of its properties is', () => {
    const graph = graphOf({
      classes: [
        { type: 'person', entities: 1, declared: true },
        { type: 'place', entities: 1, declared: true },
      ],
      properties: [
        {
          type: 'born_in',
          source: 'person',
          target: 'place',
          relations: 1,
          declared: true,
        },
        {
          type: 'hails_from',
          source: 'person',
          target: 'place',
          relations: 1,
          declared: false,
        },
      ],
    });

    const hidden = buildOntologyGraphData(graph);
    expect(hidden.links).toHaveLength(1);
    expect(hidden.links[0].types).toEqual(['born_in']);
    expect(hidden.links[0].declared).toBe(true);
    expect(hidden.links[0].undeclaredCount).toBe(0);

    const shown = buildOntologyGraphData(graph, { showUndeclared: true });
    expect(shown.links).toHaveLength(1);
    expect(shown.links[0].types).toEqual(['born_in', 'hails_from']);
    expect(shown.links[0].declared).toBe(false);
    expect(shown.links[0].undeclaredCount).toBe(1);
  });

  // Vocabulary drift is what turns the canvas into a pile when the model
  // invents its own vocabulary, so it is off by default and one toggle away.
  it('hides undeclared classes and edges until asked to show them', () => {
    const graph = graphOf({
      classes: [
        { type: 'person', label: 'Person', entities: 2, declared: true },
        // Declared by the data, not by the template.
        { type: 'franchise', entities: 3, declared: false },
      ],
      properties: [
        {
          type: 'born_in',
          source: 'person',
          target: 'person',
          relations: 1,
          declared: true,
        },
        {
          type: 'set_in',
          source: 'person',
          target: 'franchise',
          relations: 3,
          declared: false,
        },
      ],
      inheritance: [{ source: 'person', target: 'person' }],
    });

    const hidden = buildOntologyGraphData(graph);
    expect(hidden.nodes.map((node) => node.id)).toEqual(['person']);
    // The edge to the hidden class must go too: a link to a node that is not
    // drawn would be painted as a line to the origin.
    expect(
      hidden.links
        .filter((link) => link.kind === 'property')
        .flatMap((link) => link.types),
    ).toEqual(['born_in']);
    // Inheritance is a template statement, so it is never "undeclared" and it
    // survives the filter.
    expect(hidden.links.filter((link) => link.kind === 'inheritance')).toHaveLength(
      1,
    );

    const shown = buildOntologyGraphData(graph, { showUndeclared: true });
    expect(shown.nodes).toHaveLength(2);
    expect(shown.links.filter((link) => link.kind === 'property')).toHaveLength(
      2,
    );
  });

  it('skips an edge whose endpoint is not a drawn class', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [{ type: 'person', entities: 1, declared: true }],
        properties: [
          {
            type: 'born_in',
            source: 'person',
            target: 'nowhere',
            relations: 2,
            declared: true,
          },
        ],
        // The server already drops an undeclared parent, but a hand-built
        // payload must not produce a link to the origin.
        inheritance: [{ source: 'ghost', target: 'person' }],
      }),
    );

    expect(data.links).toHaveLength(0);
  });

  it('scales a class radius by its instance count and flags an empty class', () => {
    const data = buildOntologyGraphData(
      graphOf({
        classes: [
          { type: 'person', entities: 100, declared: true },
          { type: 'taxon', entities: 1, declared: true },
          { type: 'concept', entities: 0, declared: true },
        ],
      }),
    );

    const [person, taxon, concept] = data.nodes;
    expect(person.radius).toBeGreaterThan(taxon.radius);
    // 0 and 1 instance both sit on the scale's floor: the size means "this class
    // is substantial", not "this class has more than nothing".
    expect(taxon.radius).toBe(concept.radius);
    expect(data.hasEmptyClass).toBe(true);
    expect(data.maxEntities).toBe(100);
  });
});
