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

import type {
  IOntologyAttribute,
  IOntologyClass,
  IOntologyGraph,
} from '@/interfaces/database/document-structure';
import {
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  forceX,
  forceY,
} from 'd3-force';
import { type LinkObject, type NodeObject } from 'react-force-graph-2d';

export const MinClassRadius = 6;
export const MaxClassRadius = 22;

export const MinEdgeWidth = 1;
export const MaxEdgeWidth = 4;

/**
 * Offset applied to one of the two directions of a class pair, so the two
 * arrows do not sit on top of each other. Properties are bundled per ordered
 * pair, so a bundle only ever competes with its own reverse.
 */
const OppositeDirectionCurvature = 0.16;


/**
 * Inheritance edges get a fixed, modest width: rdfs:subClassOf carries no count,
 * so there is nothing to scale it by.
 */
const InheritanceEdgeWidth = 1.2;

export type OntologyGraphNode = NodeObject<IOntologyClass> & {
  id: string;
  /** What the node prints: the class's label, falling back to its name. */
  label: string;
  /** Effective datatype properties (own + inherited), from the server. */
  attributes: IOntologyAttribute[];
  /** Radius is precomputed so the canvas painter stays allocation-free. */
  radius: number;
};

/**
 * A link is either an OBJECT property edge or an rdfs:subClassOf edge. They are
 * one list because the force engine takes one, but they are styled, labelled and
 * toggled differently — which is exactly how a viewer keeps inheritance readable
 * next to the property graph.
 */
export type OntologyLinkKind = 'property' | 'inheritance';

export type OntologyGraphLink = LinkObject<
  OntologyGraphNode,
  {
    kind: OntologyLinkKind;
    /**
     * Display text. For a property bundle it is the bundled property labels joined
     * with '·' — the whole list only fits on a canvas when the bundle is small or
     * the edge is highlighted, so the painter shortens it (see the link painter)
     * and `linkLabel` carries the full list as a tooltip.
     */
    label: string;
    /**
     * The property names this link stands for. An ontology declares several
     * properties between the same two classes (`born_in`, `died_in`,
     * `citizen_of` are all person → place), and drawing one edge each turns a
     * 9-class template into 45 near-identical curves between the same pairs. One
     * link per ordered pair keeps the picture readable; the panel still lists the
     * properties individually with their own counts.
     */
    types: string[];
    /** Summed over the bundle. */
    relations: number;
    /** True only when EVERY property in the bundle is declared. */
    declared: boolean;
    /** How many of the bundled properties the template does not declare. */
    undeclaredCount: number;
    /** Signed curvature, so the two directions of a pair do not overlap. */
    curvature: number;
    width: number;
  }
>;

export interface OntologyGraphData {
  nodes: OntologyGraphNode[];
  links: OntologyGraphLink[];
  maxEntities: number;
  maxRelations: number;
  /** True when at least one class has no compiled instance in this scope. */
  hasEmptyClass: boolean;
}

/** Gap kept between the layout and the canvas edge, for the labels. */
const LayoutPadding = 44;

/** Relaxation steps. Fixed, so the same graph always lands in the same place. */
const LayoutTicks = 300;

/**
 * The most one axis may be stretched relative to the other when fitting the
 * relaxed blob to the canvas. The collision force leaves an 18px margin, and a
 * distortion of at most this much cannot eat it — a larger one could squeeze two
 * classes into each other, which is the one thing the layout guarantees.
 */
const LayoutMaxStretch = 1.35;

/** How hard a class with no edge is pulled to the centre of the canvas. */
const IsolatedCentrePull = 0.4;

/** One node inside the layout simulation. */
interface SimNode {
  id: string;
  radius: number;
  x: number;
  y: number;
}

/**
 * computeOntologyLayout spreads the classes over the whole canvas.
 *
 * It replaces a ring layout, and the reason is worth keeping: a circle is
 * inscribed by the SHORTER side of the canvas, so a wide panel wasted most of its
 * width; and on a dense ontology every chord between two classes crosses the
 * middle, so the edges and their labels piled up around the diameter — exactly
 * where a reader looks first. Neither was a matter of tuning: that is what
 * "radial" means.
 *
 * A relaxed layout is also what the reference viewer falls back to when it has no
 * saved coordinates: edges hold related classes together, repulsion and collision
 * keep them apart, and the result is then FITTED to the canvas — a relaxed blob
 * is smaller than its box, and fitting it is what turns "the graph huddles in the
 * middle" into "the graph uses the panel".
 *
 * Deterministic on purpose: the nodes start on a grid ordered by degree, and
 * d3-force only randomises positions it was not given, so the same graph always
 * produces the same picture.
 */
export const computeOntologyLayout = (
  classes: OntologyGraphNode[],
  links: OntologyGraphLink[],
  width: number,
  height: number,
): Map<string, { x: number; y: number }> => {
  const out = new Map<string, { x: number; y: number }>();
  if (classes.length === 0 || width <= 0 || height <= 0) return out;

  const degree = new Map<string, number>();
  classes.forEach((item) => degree.set(item.id, 0));
  links.forEach((link) => {
    const source = linkEndpointId(link.source);
    const target = linkEndpointId(link.target);
    if (source && degree.has(source)) {
      degree.set(source, (degree.get(source) ?? 0) + 1);
    }
    // A self-loop is one edge of the class's own neighbourhood, not two.
    if (target && target !== source && degree.has(target)) {
      degree.set(target, (degree.get(target) ?? 0) + 1);
    }
  });

  // A grid, hubs first: deterministic, and it starts the relaxation from
  // separated nodes instead of from a pile.
  const ordered = [...classes].sort(
    (a, b) =>
      (degree.get(b.id) ?? 0) - (degree.get(a.id) ?? 0) ||
      a.id.localeCompare(b.id),
  );
  const columns = Math.max(
    1,
    Math.ceil(Math.sqrt(ordered.length * (width / Math.max(height, 1)))),
  );
  const rows = Math.max(1, Math.ceil(ordered.length / columns));
  const nodes: SimNode[] = ordered.map((item, index) => ({
    id: item.id,
    radius: item.radius,
    x: ((index % columns) + 0.5) * (width / columns),
    y: (Math.floor(index / columns) + 0.5) * (height / rows),
  }));
  const edges = links.map((link) => ({
    source: linkEndpointId(link.source),
    target: linkEndpointId(link.target),
  }));

  // The two centre springs are aspect-aware: a weaker pull along x lets the graph
  // spread into a wide canvas instead of settling into a circle and leaving the
  // sides empty.
  const aspect = width / Math.max(height, 1);
  // A class with no edge of its own has nothing holding it, so the repulsion of
  // everything else walks it out to the rim — and because the layout is then
  // FITTED to the canvas, that one class also defines the bounding box: it lands
  // on the edge of the picture while every related class is squeezed inwards.
  // The shipped template did exactly this (`artifact` 538px from the cluster
  // centre while the next furthest was 317). A stronger pull to the centre puts
  // it among the others, which is the honest place for a class that relates to
  // nothing: it is unusual, not remote.
  const isolated = (id: string) => (degree.get(id) ?? 0) === 0;
  const simulation = forceSimulation<SimNode>(nodes)
    .force(
      'link',
      forceLink<SimNode, { source: string; target: string }>(edges)
        .id((node) => node.id)
        .distance(110)
        .strength(0.4),
    )
    .force('charge', forceManyBody<SimNode>().strength(-620))
    .force(
      'collide',
      forceCollide<SimNode>()
        .radius((node) => node.radius + 18)
        .strength(1),
    )
    .force(
      'x',
      forceX<SimNode>(width / 2).strength((node) =>
        isolated(node.id) ? IsolatedCentrePull : 0.05 / aspect,
      ),
    )
    .force(
      'y',
      forceY<SimNode>(height / 2).strength((node) =>
        isolated(node.id) ? IsolatedCentrePull : 0.05 * aspect,
      ),
    )
    .stop();
  for (let tick = 0; tick < LayoutTicks; tick += 1) simulation.tick();

  const xs = nodes.map((node) => node.x);
  const ys = nodes.map((node) => node.y);
  const minX = Math.min(...xs);
  const minY = Math.min(...ys);
  const spanX = Math.max(Math.max(...xs) - minX, 1);
  const spanY = Math.max(Math.max(...ys) - minY, 1);
  const targetWidth = Math.max(width - LayoutPadding * 2, 1);
  const targetHeight = Math.max(height - LayoutPadding * 2, 1);
  const scaleX = targetWidth / spanX;
  const scaleY = targetHeight / spanY;
  // Cap the relative distortion in both directions.
  const stretchX = Math.min(scaleX, scaleY * LayoutMaxStretch);
  const stretchY = Math.min(scaleY, scaleX * LayoutMaxStretch);

  nodes.forEach((node) => {
    out.set(node.id, {
      x: LayoutPadding + (node.x - minX) * stretchX,
      y: LayoutPadding + (node.y - minY) * stretchY,
    });
  });
  return out;
};

/**
 * scaleByLog compresses the count range before mapping it onto a visual size.
 * Instance counts span orders of magnitude (a hub class against a leaf class),
 * so a linear scale would collapse every leaf class into the minimum.
 */
const scaleByLog = (
  value: number,
  max: number,
  min: number,
  maxOut: number,
) => {
  if (max <= 1 || value <= 1) return min;
  const t = Math.log(value) / Math.log(max);
  return min + t * (maxOut - min);
};

/**
 * buildOntologyGraphData turns the server's class/property lists into force
 * graph data. Classes are nodes; object properties are directed domain -> range
 * edges; rdfs:subClassOf is a second, separately styled edge type.
 *
 * Datatype properties are NOT here: they have no class as their range, so there
 * is no edge to draw. They ride on the node as `attributes`, which is where a
 * viewer puts them.
 */
export const buildOntologyGraphData = (
  graph: IOntologyGraph,
  options: {
    showInheritance?: boolean;
    showUndeclared?: boolean;
    /** Canvas size, for the ring layout. 0 leaves the nodes unplaced. */
    width?: number;
    height?: number;
  } = {},
): OntologyGraphData => {
  const showInheritance = options.showInheritance !== false;
  const showUndeclared = options.showUndeclared === true;

  // Vocabulary drift (what the template does not declare) is what turns the
  // canvas into a pile when the model invents its own vocabulary, so it is
  // filtered out by default. The counts the panel shows still come from the
  // unfiltered graph — hiding a node is a view choice, not a claim that the
  // class does not exist.
  const classes = (graph.classes ?? []).filter(
    (item) => showUndeclared || item.declared !== false,
  );
  const visible = new Set(classes.map((item) => item.type));
  const properties = (graph.properties ?? []).filter(
    (item) =>
      (showUndeclared || item.declared !== false) &&
      visible.has(item.source) &&
      visible.has(item.target),
  );

  const maxEntities = classes.reduce(
    (acc, item) => Math.max(acc, item.entities ?? 0),
    0,
  );

  const nodes: OntologyGraphNode[] = classes.map((item) => ({
    // The class NAME is the node id — it is what edge endpoints and the
    // drill-down URLs are keyed by. The label is only what gets printed.
    id: item.type,
    type: item.type,
    label: item.label || item.type,
    attributes: item.attributes ?? [],
    entities: item.entities ?? 0,
    declared: item.declared !== false,
    description: item.description,
    // Carried through because the ring layout IS the hierarchy: a node stripped
    // of its parents silently collapses every class onto a single ring, which is
    // a layout bug that looks like a layout choice. It cannot be caught by the
    // type system — `parent` is optional on the DTO — so the test
    // "puts the hierarchy root in the middle and generations on rings" is what
    // guards it.
    parent: item.parent,
    radius: scaleByLog(
      item.entities ?? 0,
      maxEntities,
      MinClassRadius,
      MaxClassRadius,
    ),
  }));

  const known = new Set(nodes.map((node) => node.id));
  const links: OntologyGraphLink[] = [];

  // One bundle per ORDERED class pair. Ordered rather than unordered because
  // `member_of` (person → organization) and `led_by` (organization → person)
  // share a class pair but point opposite ways, and the direction is the only
  // thing an object property asserts — bundling across it would erase that.
  //
  // Bundling is what makes a real ontology legible: this template declares 37
  // object-property edges between 9 classes, but only 19 distinct ordered pairs,
  // so four fifths of the curves were near-duplicates between the same two
  // nodes. The panel still lists every property with its own count.
  const bundles = new Map<
    string,
    {
      source: string;
      target: string;
      types: string[];
      labels: string[];
      relations: number;
      declared: boolean;
      undeclaredCount: number;
    }
  >();
  properties.forEach((item) => {
    const { source, target } = item;
    if (!known.has(source) || !known.has(target)) return;
    const key = `${source}\u0000${target}`;
    let bundle = bundles.get(key);
    if (!bundle) {
      bundle = {
        source,
        target,
        types: [],
        labels: [],
        relations: 0,
        declared: true,
        undeclaredCount: 0,
      };
      bundles.set(key, bundle);
    }
    // The server returns properties in template declaration order, so a bundle's
    // label is stable from run to run.
    bundle.types.push(item.type);
    bundle.labels.push(item.label || item.type);
    bundle.relations += item.relations ?? 0;
    if (item.declared === false) {
      bundle.declared = false;
      bundle.undeclaredCount += 1;
    }
  });

  let maxBundleRelations = 0;
  bundles.forEach((bundle) => {
    maxBundleRelations = Math.max(maxBundleRelations, bundle.relations);
  });

  // A pair that is declared in both directions gets one offset each, so the two
  // arrows stay distinguishable instead of covering each other.
  const directionCounts = new Map<string, number>();
  bundles.forEach((bundle) => {
    if (bundle.source === bundle.target) return;
    const pair = [bundle.source, bundle.target].sort().join('\u0000');
    directionCounts.set(pair, (directionCounts.get(pair) ?? 0) + 1);
  });

  bundles.forEach((bundle) => {
    const isSelfLoop = bundle.source === bundle.target;
    let curvature = 0;
    if (!isSelfLoop) {
      const pair = [bundle.source, bundle.target].sort().join('\u0000');
      if ((directionCounts.get(pair) ?? 0) > 1) {
        // Both directions of a pair must bow the OTHER way from each other, and
        // they must agree on which way is whose — so the choice is derived from
        // the sorted pair, not from iteration order.
        const [first] = [bundle.source, bundle.target].sort();
        curvature =
          bundle.source === first
            ? OppositeDirectionCurvature
            : -OppositeDirectionCurvature;
      }
    }
    links.push({
      source: bundle.source,
      target: bundle.target,
      kind: 'property',
      label: bundle.labels.join(' · '),
      types: bundle.types,
      relations: bundle.relations,
      declared: bundle.declared,
      undeclaredCount: bundle.undeclaredCount,
      curvature,
      width: scaleByLog(
        bundle.relations,
        maxBundleRelations,
        MinEdgeWidth,
        MaxEdgeWidth,
      ),
    });
  });

  if (showInheritance) {
    (graph.inheritance ?? []).forEach((item) => {
      // The server already drops an edge whose parent is not a declared class.
      // The guard stays because a node the painter cannot resolve would be
      // drawn as a line to the origin — and because hiding undeclared classes
      // can remove an endpoint the edge list still names.
      if (!known.has(item.source) || !known.has(item.target)) return;
      links.push({
        source: item.source,
        target: item.target,
        kind: 'inheritance',
        // rdfs:subClassOf carries no vocabulary and no count, so it gets neither
        // a label nor a bundle — the dashed style is what identifies it (mirrors
        // the viewer this design follows).
        label: '',
        types: [],
        relations: 0,
        declared: true,
        undeclaredCount: 0,
        curvature: 0,
        width: InheritanceEdgeWidth,
      });
    });
  }

  // Ring layout, applied as fixed positions (`fx`/`fy`) rather than as initial
  // ones. A graph whose nodes are pinned cannot be rearranged by the simulation,
  // which is the point: the ontology is drawn the same way every time, and a
  // class with no property edge keeps the same distance from its neighbours as
  // every other class on its ring.
  const byId = new Map(nodes.map((node) => [node.id, node]));
  const layout = computeOntologyLayout(
    nodes,
    links,
    options.width ?? 0,
    options.height ?? 0,
  );
  layout.forEach((point, id) => {
    const node = byId.get(id);
    if (!node) return;
    // Start position, NOT a pin. `fx`/`fy` is a hard constraint — with it set the
    // reader cannot move the node at all, and on a dense ontology dragging a
    // class out of the crowd is how one reads it. `x`/`y` says where the layout
    // puts it; the forces are removed on the canvas side, so nothing moves it
    // until it is dragged.
    node.x = point.x;
    node.y = point.y;
    node.fx = undefined;
    node.fy = undefined;
  });

  return {
    nodes,
    links,
    maxEntities,
    // The scale is taken from the bundles that are actually drawn, so the widest
    // edge on screen is the widest bundle and not a single property's count.
    maxRelations: maxBundleRelations,
    hasEmptyClass: nodes.some((node) => node.entities === 0),
  };
};

/**
 * linkEndpointId resolves one end of a link. The force engine replaces the
 * string endpoints with node references in place, so both shapes must be read.
 */
export const linkEndpointId = (
  endpoint: OntologyGraphLink['source'],
): string => {
  if (endpoint && typeof endpoint === 'object') {
    return String((endpoint as OntologyGraphNode).id ?? '');
  }
  return endpoint === undefined || endpoint === null ? '' : String(endpoint);
};
