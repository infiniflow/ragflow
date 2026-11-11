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

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { useContainerDimensions } from '@/components/artifact-force-graph/use-container-dimensions';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
  ACTION_CONTRACT_FIELD_ORDER,
  ActionContractFieldName,
} from '@/interfaces/database/compilation-model';
import { cn } from '@/lib/utils';
import { declaredContractFields } from '@/utils/action-contract';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '../schema';
import {
  OntologyDatatypeKinds,
  OntologyKind,
  addEntity,
  addProperty,
  canonicalProperties,
  draftFromSections,
  effectiveKind,
  removeEntity,
  removeProperty,
  renameEntity,
  sectionsFromDraft,
  valueListText,
  valueListTruthy,
  splitTypeList,
  updateField,
  validateDraft,
  type OntologyDraft,
  type OntologyField,
  type OntologySection,
} from './model';

/** Canvas geometry. Fixed cells keep the picture still while the graph is edited. */
const NodeWidth = 200;
const NodeHeaderHeight = 52;
const AttributeRowHeight = 16;
const CellWidth = 260;
const CellHeight = 180;
const CanvasPadding = 40;
/** Kept between a connector and any box it must not touch. */
const RouteMargin = 18;
/**
 * How far above its segment an edge label sits. The link handle on a node's right
 * edge occupies a band from 16px to 36px below the box top, and the 60px between
 * two columns is where labels live too — 6px of lift was inside that band, which is
 * why a label could sit on the handle.
 */
const EdgeLabelLift = 14;
/** The detail panel starts wider than a sidebar: it is a form, not a legend, and
 *  the canvas used to carry the whole width by default. */
const DefaultPanelWidth = 320;
const MinPanelWidth = 260;
const MinCanvasWidth = 280;
/** One arrow-key press. */
const PanelResizeStep = 16;

/**
 * What a resize handler needs from its event. Deliberately structural, because the
 * divider listens to BOTH pointer and mouse events: jsdom has no `PointerEvent` at
 * all, so a pointer-only divider cannot be driven by a test — and a pointer-only
 * control is the one piece of drag behaviour that would then never be covered.
 * Both families firing for one gesture is harmless here: the handlers compute the
 * same width from the same coordinates, so the second call is a no-op.
 */
type ResizeEvent = {
  clientX: number;
  pointerId?: number;
  currentTarget: EventTarget & {
    setPointerCapture?: (pointerId: number) => void;
  };
  preventDefault: () => void;
};

/** How far a self-loop stands off its node. It goes UP, not out to the right: the
 *  right edge is where the link handle lives, and the 60px between two columns
 *  cannot hold a loop, a label and a control at once. Above the box is empty. */
const LoopBulge = 34;

type NodeBox = {
  left: number;
  top: number;
  right: number;
  bottom: number;
  cx: number;
  cy: number;
};

/**
 * The rectangle an entity occupies. Its height grows with its attribute rows, so the
 * router and the drawn `rect` read the same number — a box the router thinks is
 * smaller than it is puts connectors through its rows.
 */
const nodeBox = (node: {
  x: number;
  y: number;
  attributes: unknown[];
}): NodeBox => {
  const height = NodeHeaderHeight + node.attributes.length * AttributeRowHeight;
  return {
    left: node.x,
    top: node.y,
    right: node.x + NodeWidth,
    bottom: node.y + height,
    cx: node.x + NodeWidth / 2,
    cy: node.y + height / 2,
  };
};

/**
 * Where a line leaving the box's centre toward `target` crosses its edge. A
 * connector drawn to the centre pokes into the box and puts the arrowhead inside
 * it; anchoring on the border is what makes the arrow point AT the entity.
 */
const borderPoint = (box: NodeBox, target: { x: number; y: number }) => {
  const dx = target.x - box.cx;
  const dy = target.y - box.cy;
  // The box's OWN dimensions, not the module constant: the router transposes the
  // plane for vertical pairs, and a helper that assumes a fixed width breaks the
  // moment it is handed swapped coordinates.
  const scale = Math.min(
    dx === 0
      ? Number.POSITIVE_INFINITY
      : (box.right - box.left) / 2 / Math.abs(dx),
    dy === 0
      ? Number.POSITIVE_INFINITY
      : (box.bottom - box.top) / 2 / Math.abs(dy),
  );
  if (!Number.isFinite(scale)) return { x: box.cx, y: box.cy };
  return { x: box.cx + dx * scale, y: box.cy + dy * scale };
};

/** Segment vs rectangle (Liang-Barsky): the router's only obstacle test. */
const segmentHitsBox = (
  a: { x: number; y: number },
  b: { x: number; y: number },
  box: NodeBox,
) => {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  let enter = 0;
  let leave = 1;
  const clip = (p: number, q: number) => {
    if (p === 0) return q >= 0;
    const r = q / p;
    if (p < 0) {
      if (r > leave) return false;
      if (r > enter) enter = r;
    } else {
      if (r < enter) return false;
      if (r < leave) leave = r;
    }
    return true;
  };
  return (
    clip(-dx, a.x - box.left) &&
    clip(dx, box.right - a.x) &&
    clip(-dy, a.y - box.top) &&
    clip(dy, box.bottom - a.y)
  );
};

/**
 * Orthogonal (Manhattan) routing: every segment is horizontal or vertical, so every
 * bend is a right angle. A diagonal detour is shorter, but beside a grid of boxes
 * it reads as a scribble — which is what it was reported as.
 *
 * The whole thing is written for a left-to-right pair; a top-to-bottom pair is the
 * same problem with the axes swapped, so `routeEdge` transposes the plane, calls
 * this, and transposes back. One router, two orientations.
 */
const routeHorizontal = (
  from: NodeBox,
  to: NodeBox,
  blockers: NodeBox[],
  side: number,
) => {
  const exit = borderPoint(from, { x: to.cx, y: from.cy });
  const enter = borderPoint(to, { x: from.cx, y: to.cy });

  if (blockers.length === 0) {
    // Aligned: a single straight segment is already orthogonal.
    if (Math.abs(exit.y - enter.y) < 1) return [exit, enter];
    // Otherwise the standard Z: out, across at the middle, in.
    const mid = (exit.x + enter.x) / 2;
    return [exit, { x: mid, y: exit.y }, { x: mid, y: enter.y }, enter];
  }

  // Blocked: drop into a clear lane above (or below) every obstacle, run across it,
  // and come back. The vertical runs happen in the gaps between the columns, so
  // they never hug a box.
  const laneY =
    (side > 0
      ? Math.max(...blockers.map((box) => box.bottom))
      : Math.min(...blockers.map((box) => box.top))) +
    side * RouteMargin;
  const goingRight = to.cx >= from.cx;
  const nearEdge = goingRight
    ? Math.min(...blockers.map((box) => box.left))
    : Math.max(...blockers.map((box) => box.right));
  const farEdge = goingRight
    ? Math.max(...blockers.map((box) => box.right))
    : Math.min(...blockers.map((box) => box.left));
  const laneBefore = (exit.x + nearEdge) / 2;
  const laneAfter = (farEdge + enter.x) / 2;
  return [
    exit,
    { x: laneBefore, y: exit.y },
    { x: laneBefore, y: laneY },
    { x: laneAfter, y: laneY },
    { x: laneAfter, y: enter.y },
    enter,
  ];
};

/** Serialises a routed polyline as an SVG path. */
const toPath = (points: { x: number; y: number }[]) =>
  points
    .map((point, at) => `${at === 0 ? 'M' : 'L'}${point.x},${point.y}`)
    .join(' ');

/** Drops repeated vertices, which a collapsed Z produces. */
const dedupe = (points: { x: number; y: number }[]) =>
  points.filter(
    (point, at) =>
      at === 0 || point.x !== points[at - 1].x || point.y !== points[at - 1].y,
  );

/** The transposed box: swapping x and y turns a vertical pair into a horizontal one. */
const transposeBox = (box: NodeBox): NodeBox => ({
  left: box.top,
  right: box.bottom,
  top: box.left,
  bottom: box.right,
  cx: box.cy,
  cy: box.cx,
});

const routeEdge = (from: NodeBox, to: NodeBox, obstacles: NodeBox[]) => {
  const fromCentre = { x: from.cx, y: from.cy };
  const toCentre = { x: to.cx, y: to.cy };
  const blockers = obstacles.filter((box) =>
    segmentHitsBox(fromCentre, toCentre, box),
  );
  // Which side to pass on: whichever side the obstacles are on, measured in the
  // horizontal orientation. A row of boxes centred on the line counts as "below".
  const side =
    blockers.filter((box) => box.cy >= fromCentre.y).length >=
    blockers.length / 2
      ? 1
      : -1;
  const horizontal =
    Math.abs(toCentre.x - fromCentre.x) >= Math.abs(toCentre.y - fromCentre.y);

  const points = horizontal
    ? routeHorizontal(from, to, blockers, side)
    : routeHorizontal(
        transposeBox(from),
        transposeBox(to),
        blockers.map(transposeBox),
        side,
      ).map((point) => ({ x: point.y, y: point.x }));

  const deduped = dedupe(points);
  // The label rides the middle segment, which is where the room is.
  const middle = (deduped.length - 1) / 2;
  const a = deduped[Math.floor(middle)];
  const b = deduped[Math.ceil(middle)];
  return { points: deduped, label: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } };
};

/**
 * Where the draggable arrowhead sits: a little back along the incoming segment, so
 * the grip does not cover the arrowhead it is dragging, and so it stays clickable
 * when the edge is one pixel wide.
 */
const endpointHandle = (
  points: { x: number; y: number }[],
  back = 10,
): { x: number; y: number } => {
  const last = points[points.length - 1] ?? { x: 0, y: 0 };
  const prev = points[points.length - 2] ?? last;
  const dx = last.x - prev.x;
  const dy = last.y - prev.y;
  const length = Math.hypot(dx, dy);
  if (length === 0) return last;
  return { x: last.x - (dx / length) * back, y: last.y - (dy / length) * back };
};

/**
 * Placed on a grid, in declaration order: adding an entity never moves the others.
 */
export const nodePosition = (index: number, columns: number) => ({
  x: CanvasPadding + (index % columns) * CellWidth,
  y: CanvasPadding + Math.floor(index / columns) * CellHeight,
});

export const gridColumns = (count: number) =>
  Math.max(
    1,
    Math.min(Math.max(count, 1), Math.ceil(Math.sqrt(Math.max(count, 1))) + 1),
  );

/**
 * uniquePropertyName keeps the auto-generated name of a new property unique: the
 * service rejects two properties with the same name, and the reader is going to
 * rename it anyway.
 */
const uniquePropertyName = (draft: OntologyDraft, base: string): string => {
  const taken = new Set(
    draft.properties.map((item) => String(item.type ?? '').trim()),
  );
  if (!taken.has(base)) return base;
  let suffix = 2;
  while (taken.has(`${base}_${suffix}`)) suffix += 1;
  return `${base}_${suffix}`;
};

/**
 * issueLabel translates a validation code, falling back to the message the model
 * composed. The model already reads a whole template and its sentences are
 * English; a locale that has not translated a code still shows something true,
 * and the codes stay the single source of truth for what can go wrong.
 */
const issueLabel = (
  issue: { code: string; message: string },
  translate: (key: string, options: Record<string, unknown>) => string,
): string =>
  translate(`knowledgeCompilation.ontologyIssue_${issue.code}`, {
    defaultValue: issue.message,
  });

/**
 * contractFieldLabel names one of the six contract fields.
 *
 * The same keys the review card uses: the declaration edited here and the values
 * judged there are the same six things, and two sets of labels would let them
 * drift apart in the UI while matching in the data.
 */
const contractFieldLabel = (
  name: ActionContractFieldName,
  translate: (key: string, options: Record<string, unknown>) => string,
): string =>
  translate(
    `knowledgeCompilation.ontologyInspect.actionContractField.${name}`,
    {
      defaultValue: name,
    },
  );

interface OntologyCanvasEditorProps {
  form: UseFormReturn<FormSchemaType>;
  templateIndex: number;
}

type Selection =
  | { kind: 'entity'; index: number }
  | { kind: 'property'; index: number }
  // A pair of entities can carry several properties. They are drawn as ONE edge:
  // N curves with N labels between two boxes is a thicket, and on a short edge the
  // labels cannot be kept apart — the midpoint of a Bézier only moves half as far
  // as its control point, so a 40px fan leaves 20px between two 11px labels.
  // Clicking the edge therefore selects the bundle, and the panel lists what is
  // inside it. The read-only model graph bundles the same way.
  | { kind: 'bundle'; key: string }
  | null;

/**
 * OntologyCanvasEditor edits an ontology template as a graph.
 *
 * It is the ontology counterpart of the card grid: entities are nodes, object
 * properties are the arrows between them, and an entity's datatype properties are
 * listed on the node — the same picture the read-only model graph draws, with
 * every element now creatable, renamable, bindable and deletable.
 *
 * The canvas is a VIEW. Every edit is turned into the template's own field
 * arrays by `model.ts` and written straight back into the form, so saving goes
 * through the unchanged path
 * (`transformFormToPayload` → `/v1/compilation-template-groups`).
 *
 * Node positions are session-only ON PURPOSE: the template has no position
 * field, and adding one would break "the canvas saves the template format".
 */
export function OntologyCanvasEditor({
  form,
  templateIndex,
}: OntologyCanvasEditorProps) {
  const { t } = useTranslation();
  // `as const` keeps react-hook-form's path types: a plain concatenated string
  // widens to `string` and no longer matches the form's path union.
  const paths = {
    entity: `templates.${templateIndex}.config.entity`,
    relation: `templates.${templateIndex}.config.relation`,
  } as const;
  const entity = useWatch({
    control: form.control,
    name: paths.entity,
  }) as OntologySection | undefined;
  const relation = useWatch({
    control: form.control,
    name: paths.relation,
  }) as OntologySection | undefined;

  const draft = useMemo(
    () => draftFromSections(entity, relation),
    [entity, relation],
  );
  const [selection, setSelection] = useState<Selection>(null);
  const [positions, setPositions] = useState<
    Record<string, { x: number; y: number }>
  >({});
  const dragging = useRef<{ id: string; x: number; y: number } | null>(null);
  // What the left rail is listing. A model is made of entities, relations and the
  // rules they must satisfy, and one flat entity list made the other two invisible:
  // a relation could only be reached by clicking its label on the canvas, and a
  // declared problem could not be reached at all.
  const [railTab, setRailTab] = useState<'entities' | 'relations' | 'rules'>(
    'entities',
  );
  // Dragging from an entity's edge anchor to another entity creates the relation.
  // The anchor only appears on hover, so the canvas stays quiet, and the drag is
  // the one gesture a reader already expects from a graph editor — the "+" chip
  // that used to sit on every node was reported as an unexplained button.
  const [linking, setLinking] = useState<{
    from: string;
    x: number;
    y: number;
  } | null>(null);
  // Dragging the arrowhead itself: `from` is the domain end, which stays where it
  // is, and the pointer says where the range end lands. Separate from `linking`
  // because the gesture rewrites an existing declaration instead of adding one.
  const [rewiring, setRewiring] = useState<{
    propertyIndex: number;
    from: string;
    x: number;
    y: number;
  } | null>(null);
  // The pointerup that finishes a link is followed by a click on the same node,
  // which would otherwise move the selection off the relation just created.
  const justLinked = useRef(false);
  // The same is true of an arrowhead dropped on a node: the click belongs to the
  // drag, not to the entity that happened to receive it.
  const justRewired = useRef(false);

  useEffect(() => {
    if (!linking && !rewiring) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      setLinking(null);
      setRewiring(null);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [linking, rewiring]);
  const [panelWidth, setPanelWidth] = useState(DefaultPanelWidth);
  const resizing = useRef<{ x: number; width: number; limit: number } | null>(
    null,
  );

  const beginResize = (event: ResizeEvent) => {
    // The canvas' own width is the room the drag must leave alone. It is measured
    // when the drag starts because it changes as the drag proceeds.
    resizing.current = {
      x: event.clientX,
      width: panelWidth,
      limit: panelWidth + canvasBox.width - MinCanvasWidth,
    };
    try {
      event.currentTarget.setPointerCapture?.(event.pointerId ?? 0);
    } catch {
      // jsdom has no pointer capture; the drag works without it in a browser.
    }
    event.preventDefault();
  };

  const applyResize = (event: ResizeEvent) => {
    const active = resizing.current;
    if (!active) return;
    const next = active.width - (event.clientX - active.x);
    // The ceiling is what keeps `MinCanvasWidth` for the canvas, and it only means
    // anything once the canvas has actually been measured. A zero-width reading
    // must not be read as "the canvas is full", or the panel could not be widened
    // at all — which is exactly what a test environment reports.
    const ceiling =
      active.limit >= MinPanelWidth ? active.limit : Number.POSITIVE_INFINITY;
    setPanelWidth(Math.max(MinPanelWidth, Math.min(next, ceiling)));
  };

  const endResize = () => {
    resizing.current = null;
  };

  const nudgePanel = (delta: number) =>
    setPanelWidth((current) => Math.max(MinPanelWidth, current + delta));

  const properties = useMemo(
    () => canonicalProperties(draft.properties),
    [draft],
  );
  const entitySet = useMemo(
    () =>
      new Set(
        draft.entities
          .map((item) => String(item.type ?? '').trim())
          .filter(Boolean),
      ),
    [draft],
  );
  const issues = useMemo(() => validateDraft(draft), [draft]);

  const write = useCallback(
    (next: OntologyDraft) => {
      // The sections as they stand are handed back in, so whatever the editor
      // does not own (`entity.output_fields`, a section description) survives
      // the write.
      const sections = sectionsFromDraft(next, { entity, relation });
      form.setValue(paths.entity, sections.entity, { shouldDirty: true });
      form.setValue(paths.relation, sections.relation, {
        shouldDirty: true,
      });
    },
    [entity, form, paths.entity, paths.relation, relation],
  );

  const editClass = useCallback(
    (index: number, patch: OntologyField) => {
      // A rename has to rewrite the references, so it goes through the model
      // instead of patching the field in place.
      if (patch.type !== undefined) {
        write(renameEntity(draft, index, patch.type));
        return;
      }
      write({ ...draft, entities: updateField(draft.entities, index, patch) });
    },
    [draft, write],
  );

  const editProperty = useCallback(
    (index: number, patch: OntologyField) => {
      write({
        ...draft,
        properties: updateField(draft.properties, index, patch),
      });
    },
    [draft, write],
  );

  const columns = gridColumns(draft.entities.length);
  const nodes = draft.entities.map((field, index) => {
    const id = String(field.type ?? '').trim();
    const at = positions[id] ?? nodePosition(index, columns);
    return {
      id,
      label: String(field.label ?? '').trim() || id,
      parents: splitTypeList(field.parent).filter((parent) =>
        entitySet.has(parent),
      ),
      attributes: properties
        .filter(
          (property) =>
            effectiveKind(property, entitySet) === OntologyKind.Datatype &&
            splitTypeList(property.domain).includes(id),
        )
        .map((property) => ({
          name: String(property.type ?? '').trim(),
          datatype: String(property.datatype ?? '').trim(),
        }))
        .filter((attribute) => attribute.name),
      x: at.x,
      y: at.y,
    };
  });
  const byId = new Map(nodes.map((node) => [node.id, node]));

  const edges = (() => {
    const grouped = new Map<
      string,
      {
        key: string;
        from: (typeof nodes)[number];
        to: (typeof nodes)[number];
        loop: boolean;
        items: { index: number; label: string; forward: boolean }[];
      }
    >();
    properties.forEach((property, index) => {
      if (effectiveKind(property, entitySet) !== OntologyKind.Object) return;
      const domains = splitTypeList(property.domain);
      const ranges = splitTypeList(property.range);
      // A property that declares several endpoints has no single edge to draw:
      // taking the first of each list invented a self-loop out of `domain:
      // agent|place` + `range: agent|place`, which is not what the declaration
      // says. It is listed in the Relations tab with its full endpoint lists
      // instead, and the toolbar says how many were left off.
      if (domains.length !== 1 || ranges.length !== 1) return;
      const fromId = domains[0];
      const toId = ranges[0];
      const from = byId.get(fromId);
      const to = byId.get(toId);
      if (!from || !to) return;
      // Keyed by the UNORDERED pair: `a -> b` and `b -> a` between the same two
      // entities are one visual edge. Drawing them as two opposing curves is part
      // of what made the picture unreadable.
      const key = [fromId, toId].sort().join('|');
      const entry = grouped.get(key) ?? {
        key,
        from,
        to,
        loop: fromId === toId,
        items: [],
      };
      entry.items.push({
        index,
        label:
          String(property.label ?? '').trim() ||
          String(property.type ?? '').trim(),
        // Direction is per property, so a bundle has to be able to say "both
        // ways". An arrowhead on one end only would be a lie about the other
        // property in the bundle.
        forward: from.id === entry.from.id,
      });
      grouped.set(key, entry);
    });
    return [...grouped.values()].map((entry) => ({
      ...entry,
      fromBox: nodeBox(entry.from),
      toBox: nodeBox(entry.to),
      // A self-loop has no straight line to route, so it keeps its own geometry in
      // the drawing block.
      route: entry.loop
        ? null
        : routeEdge(
            nodeBox(entry.from),
            nodeBox(entry.to),
            nodes
              .filter(
                (node) => node.id !== entry.from.id && node.id !== entry.to.id,
              )
              .map((node) => nodeBox(node)),
          ),
      // A lone property keeps its own name as the test id and the label: that is
      // the common case and it must not regress into "1 property".
      id:
        entry.items.length === 1
          ? String(properties[entry.items[0].index]?.type ?? '').trim()
          : `bundle-${entry.key}`,
      // A bare "×3" says a number, not a relation. The first property names the
      // edge and the count says how many more share it; the title lists them.
      text:
        entry.items.length === 1
          ? entry.items[0].label
          : `${entry.items[0].label} +${entry.items.length - 1}`,
      mixed:
        entry.items.some((item) => item.forward) &&
        entry.items.some((item) => !item.forward),
    }));
  })();

  const rows = Math.ceil(Math.max(draft.entities.length, 1) / columns);
  const contentWidth = CanvasPadding * 2 + Math.max(columns, 1) * CellWidth;
  const contentHeight = CanvasPadding * 2 + rows * CellHeight;

  // The canvas is as big as the space it was given, and no smaller than its
  // content: the grid still decides where the entities ARE (positions come from
  // `nodePosition`, not from this box), so measurement only ever adds empty
  // room to drag into — it can never move an entity.
  const canvasBoxRef = useRef<HTMLDivElement>(null);
  const canvasBox = useContainerDimensions(canvasBoxRef);
  const canvasWidth = Math.max(contentWidth, canvasBox.width);
  const canvasHeight = Math.max(contentHeight, canvasBox.height);

  const entityIndex = selection?.kind === 'entity' ? selection.index : -1;
  const propertyIndex = selection?.kind === 'property' ? selection.index : -1;
  const selectedEntity = draft.entities[entityIndex];
  const selectedProperty = properties[propertyIndex];
  const selectedKind =
    selectedProperty === undefined
      ? ''
      : String(selectedProperty.kind ?? '').trim() ||
        effectiveKind(selectedProperty, entitySet);
  const selectedIssues = issues.filter(
    (issue) =>
      issue.subject !== '' &&
      issue.subject ===
        String((selectedEntity ?? selectedProperty)?.type ?? '').trim(),
  );

  // Which properties belong to the selected entity. A property created with no
  // domain was findable nowhere — this list is one of the two answers to "which
  // entity does this belong to"; the attribute rows drawn on the node are the
  // other, and both come from the same declaration.
  const selectedEntityType = String(selectedEntity?.type ?? '').trim();
  const ownProperties = selectedEntity
    ? properties
        .map((property, index) => ({ property, index }))
        .filter(({ property }) =>
          splitTypeList(property.domain).includes(selectedEntityType),
        )
        .map(({ property, index }) => ({
          index,
          label:
            String(property.label ?? '').trim() ||
            String(property.type ?? '').trim(),
          target:
            effectiveKind(property, entitySet) === OntologyKind.Datatype
              ? ''
              : (splitTypeList(property.range)[0] ?? ''),
        }))
    : [];

  // A finding names an entity or a property, so it can take the reader there. This is
  // the ONLY way to reach a property with no domain: it draws no edge, so it cannot
  // be clicked on the canvas, and it is in no entity's list either.
  const selectBySubject = (subject: string) => {
    const classAt = draft.entities.findIndex(
      (item) => String(item.type ?? '').trim() === subject,
    );
    if (classAt >= 0) {
      setSelection({ kind: 'entity', index: classAt });
      return;
    }
    const propertyAt = properties.findIndex(
      (item) => String(item.type ?? '').trim() === subject,
    );
    if (propertyAt >= 0) setSelection({ kind: 'property', index: propertyAt });
  };

  const selectedBundle =
    selection?.kind === 'bundle'
      ? edges.find((edge) => edge.key === selection.key)
      : undefined;

  const selectedPropertyDomain =
    splitTypeList(selectedProperty?.domain)[0] ?? '';
  const selectedPropertyRange = splitTypeList(selectedProperty?.range)[0] ?? '';

  // Anchored on creation: the domain is the entity the reader is looking at, so the
  // new attribute shows up on that node immediately and can never be an orphan.
  // That is also the state the reference implementation's form enforces — its
  // domain and range are both `required`.
  const addOwnAttribute = () => {
    const next = addProperty(draft, OntologyKind.Datatype);
    const created = next.properties.length - 1;
    write({
      ...next,
      properties: updateField(next.properties, created, {
        domain: selectedEntityType,
      }),
    });
    setSelection({ kind: 'property', index: created });
  };

  // What the Relations tab lists: every object property, with the two ends it
  // declares. An object property with no domain draws no edge and is reachable
  // nowhere else, which is exactly why it belongs in a list of its own.
  const objectProperties = properties
    .map((property, index) => {
      const domains = splitTypeList(property.domain);
      const ranges = splitTypeList(property.range);
      return {
        index,
        name: String(property.type ?? '').trim(),
        // Both lists in full: the canvas can only draw a single pair, and a row
        // that printed the first of each would disagree with the declaration.
        domain: domains.join(' | '),
        range: ranges.join(' | '),
        kind: effectiveKind(property, entitySet),
        polymorphic: domains.length !== 1 || ranges.length !== 1,
      };
    })
    .filter(
      (property) =>
        property.name !== '' && property.kind !== OntologyKind.Datatype,
    );

  // Every relation the canvas cannot draw as one edge: several declared
  // domains or ranges, or an endpoint that is not a declared entity. They are
  // reachable in the Relations tab, which prints both lists in full.
  const polymorphicProperties = properties.filter((property) => {
    if (effectiveKind(property, entitySet) !== OntologyKind.Object)
      return false;
    const domains = splitTypeList(property.domain);
    const ranges = splitTypeList(property.range);
    if (domains.length === 1 && ranges.length === 1) {
      return !entitySet.has(domains[0]) || !entitySet.has(ranges[0]);
    }
    return true;
  });

  const svgPoint = (event: {
    clientX: number;
    clientY: number;
    currentTarget: EventTarget | null;
  }) => {
    const target = event.currentTarget as (SVGElement & Element) | null;
    const svg = target?.ownerSVGElement ?? target?.closest?.('svg') ?? null;
    const rect = svg?.getBoundingClientRect();
    return {
      x: event.clientX - (rect?.left ?? 0),
      y: event.clientY - (rect?.top ?? 0),
    };
  };

  const startLink = (
    from: string,
    event: {
      clientX: number;
      clientY: number;
      currentTarget: EventTarget | null;
      stopPropagation: () => void;
    },
  ) => {
    // Without this the pointerdown also starts dragging the node.
    event.stopPropagation();
    const point = svgPoint(event);
    setLinking({ from, x: point.x, y: point.y });
  };

  // The declaration the drag writes: one object property, domain = where the drag
  // started, range = where it ended. Same shape the Relations tab writes, so both
  // entry points produce a template the service accepts.
  const finishLink = (to: string) => {
    const from = linking?.from;
    setLinking(null);
    if (!from) return;
    const next = addProperty(draft, OntologyKind.Object);
    const created = next.properties.length - 1;
    write({
      ...next,
      properties: updateField(next.properties, created, {
        type: uniquePropertyName(next, `${from}_${to}`),
        domain: from,
        range: to,
      }),
    });
    justLinked.current = true;
    setSelection({ kind: 'property', index: created });
  };

  const startRewire = (
    propertyIndex: number,
    from: string,
    event: {
      clientX: number;
      clientY: number;
      currentTarget: EventTarget | null;
      stopPropagation: () => void;
    },
  ) => {
    // Without this the pointerdown also drags the node the grip sits on.
    event.stopPropagation();
    const point = svgPoint(event);
    setRewiring({ propertyIndex, from, x: point.x, y: point.y });
  };

  // Dropping the arrowhead on an entity rewrites the property's range — including
  // the entity the property already starts from, which is how a relation is made
  // to point at itself. The domain end never moves: a self-loop drags one end onto
  // the other, it does not swap them.
  const finishRewire = (to: string) => {
    const active = rewiring;
    setRewiring(null);
    if (!active) return;
    editProperty(active.propertyIndex, { range: to });
    justRewired.current = true;
    // The click after the drop belongs to the drag. It may arrive on the entity
    // (and would move the selection off the property) or on the canvas, so the
    // flag is cleared on the next task instead of waiting for a click that might
    // never come — otherwise the NEXT click on an entity would be swallowed.
    window.setTimeout(() => {
      justRewired.current = false;
    }, 0);
    setSelection({ kind: 'property', index: active.propertyIndex });
  };

  const handleAddEntity = () => {
    const next = addEntity(draft);
    write(next);
    setSelection({ kind: 'entity', index: next.entities.length - 1 });
  };

  // Relations start from the list, not from a handle on the node: the handle was
  // reported as an unexplained "+", and a list states both ends before anything
  // exists. The first two entities are used as the ends so the new relation is
  // visible on the canvas immediately and is never an orphan.
  const handleAddRelation = () => {
    const next = addProperty(draft, OntologyKind.Object);
    const created = next.properties.length - 1;
    const source = String(next.entities[0]?.type ?? '').trim();
    const target = String(next.entities[1]?.type ?? '').trim() || source;
    write({
      ...next,
      properties: updateField(next.properties, created, {
        type: uniquePropertyName(
          next,
          source && target ? `${source}_${target}` : 'relation',
        ),
        domain: source,
        range: target,
      }),
    });
    setSelection({ kind: 'property', index: created });
  };

  const entityOptions = [...entitySet]
    .sort()
    .map((name) => ({ label: name, value: name }));

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setPositions({})}
        >
          {t('knowledgeCompilation.ontologyAutoArrange', {
            defaultValue: 'Auto arrange',
          })}
        </Button>
        {polymorphicProperties.length > 0 && (
          <span
            className="text-xs text-text-disabled"
            data-testid="ontology-polymorphic-notice"
          >
            {t('knowledgeCompilation.ontologyPolymorphicHidden', {
              count: polymorphicProperties.length,
              defaultValue:
                '{{count}} relation(s) span several entities, so they are listed in Relations instead of drawn here.',
            })}
          </span>
        )}
        <span className="text-xs text-text-secondary">
          {t('knowledgeCompilation.ontologyDragHint', {
            defaultValue:
              'Drag an entity to move it; drag its edge dot onto another entity to connect them. Positions are not stored in the template.',
          })}
        </span>
        {/* Two line styles, two meanings. Without this the dashed parent edge
            reads as a second kind of relation. */}
        <span className="ml-auto flex items-center gap-3 text-xs text-text-secondary">
          <span className="flex items-center gap-1">
            <svg width="22" height="8" aria-hidden="true">
              <line
                x1="0"
                y1="4"
                x2="22"
                y2="4"
                stroke="currentColor"
                strokeWidth="1.5"
                markerEnd="url(#ontology-arrow)"
              />
            </svg>
            {t('knowledgeCompilation.ontologyLegendRelation', {
              defaultValue: 'Relation',
            })}
          </span>
          <span className="flex items-center gap-1">
            <svg width="22" height="8" aria-hidden="true">
              <line
                x1="0"
                y1="4"
                x2="22"
                y2="4"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeDasharray="4 3"
              />
            </svg>
            {t('knowledgeCompilation.ontologyLegendInheritance', {
              defaultValue: 'Inheritance (parent)',
            })}
          </span>
        </span>
      </div>

      <div className="flex min-h-0 flex-1 gap-3">
        {/* Three columns: what exists (260), what it looks like (flex), what the
            selected thing says (320, resizable). On a narrow window the left
            column folds away — the canvas is the workspace here, and a list that
            squeezes it earns less than the scroll it saves. */}
        <aside
          data-testid="ontology-entity-list"
          className="hidden w-[260px] shrink-0 flex-col gap-2 overflow-auto rounded border border-border-button bg-bg-component p-2 min-[1180px]:flex"
        >
          {/* Entities / Relations / Rules: the three things a model is made of.
              One flat list made entities the only visible noun, so a relation had
              no home outside a canvas label and a declared problem had none at
              all. */}
          <div className="flex shrink-0 items-center gap-1" role="tablist">
            {(
              [
                {
                  key: 'entities',
                  label: t('knowledgeCompilation.ontologyRailEntities', {
                    defaultValue: 'Entities',
                  }),
                },
                {
                  key: 'relations',
                  label: t('knowledgeCompilation.ontologyRailRelations', {
                    defaultValue: 'Relations',
                  }),
                },
                {
                  key: 'rules',
                  label: t('knowledgeCompilation.ontologyRailRules', {
                    defaultValue: 'Rules',
                  }),
                },
              ] as const
            ).map((item) => (
              <button
                key={item.key}
                type="button"
                role="tab"
                aria-selected={railTab === item.key}
                data-testid={`ontology-rail-tab-${item.key}`}
                onClick={() => setRailTab(item.key)}
                className={cn(
                  'flex-1 rounded px-1 py-1 text-xs',
                  railTab === item.key
                    ? 'bg-bg-card font-medium text-text-primary'
                    : 'text-text-secondary hover:bg-bg-card',
                )}
              >
                {item.label}
                {item.key === 'rules' && issues.length > 0
                  ? ` (${issues.length})`
                  : ''}
              </button>
            ))}
          </div>

          {railTab === 'entities' && (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="ontology-add-entity"
                onClick={handleAddEntity}
              >
                {t('knowledgeCompilation.ontologyAddEntity', {
                  defaultValue: '+ Entity',
                })}
              </Button>
              <span className="px-1 text-xs text-text-secondary">
                {t('knowledgeCompilation.ontologyEntityList', {
                  count: nodes.length,
                  defaultValue: 'Entities ({{count}})',
                })}
              </span>
              {nodes.map((node, index) => {
                const isSelected =
                  selection?.kind === 'entity' && selection.index === index;
                return (
                  <button
                    key={`entity-list-${node.id}`}
                    type="button"
                    data-testid={`ontology-entity-list-item-${node.id}`}
                    onClick={() => setSelection({ kind: 'entity', index })}
                    className={cn(
                      'flex items-center gap-2 rounded px-2 py-1 text-start text-xs',
                      isSelected
                        ? 'bg-bg-card text-text-primary'
                        : 'text-text-secondary hover:bg-bg-card',
                    )}
                  >
                    <span className="min-w-0 flex-1 truncate">
                      {node.label}
                    </span>
                    {node.attributes.length > 0 && (
                      <span className="shrink-0 text-text-disabled">
                        {node.attributes.length}
                      </span>
                    )}
                  </button>
                );
              })}
            </>
          )}

          {railTab === 'relations' && (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="ontology-add-relation"
                onClick={handleAddRelation}
              >
                {t('knowledgeCompilation.ontologyAddRelation', {
                  defaultValue: '+ Relation',
                })}
              </Button>
              {objectProperties.length === 0 ? (
                <span className="px-1 text-xs text-text-disabled">
                  {t('knowledgeCompilation.ontologyNoRelations', {
                    defaultValue: 'No relations yet.',
                  })}
                </span>
              ) : (
                objectProperties.map((property) => {
                  const isSelected =
                    selection?.kind === 'property' &&
                    selection.index === property.index;
                  return (
                    <button
                      key={`relation-list-${property.index}`}
                      type="button"
                      data-testid={`ontology-relation-list-item-${property.name}`}
                      onClick={() =>
                        setSelection({
                          kind: 'property',
                          index: property.index,
                        })
                      }
                      className={cn(
                        'flex flex-col rounded px-2 py-1 text-start text-xs',
                        isSelected
                          ? 'bg-bg-card text-text-primary'
                          : 'text-text-secondary hover:bg-bg-card',
                      )}
                    >
                      <span className="w-full truncate">{property.name}</span>
                      <span className="w-full truncate text-text-disabled">
                        {`${property.domain || '?'} → ${property.range || '?'}`}
                      </span>
                      {property.polymorphic && (
                        <span className="w-full truncate text-text-disabled">
                          {t('knowledgeCompilation.ontologyPolymorphicRow', {
                            defaultValue: 'not drawn: several endpoints',
                          })}
                        </span>
                      )}
                    </button>
                  );
                })
              )}
            </>
          )}

          {railTab === 'rules' && (
            <>
              {issues.length === 0 ? (
                <span className="px-1 text-xs text-text-disabled">
                  {t('knowledgeCompilation.ontologyNoRules', {
                    defaultValue: 'No problems found.',
                  })}
                </span>
              ) : (
                <ul className="flex flex-col gap-1 px-1 text-xs text-[#F0A020]">
                  {issues.map((issue, at) => (
                    <li key={`${issue.code}-${issue.subject}-${at}`}>
                      <button
                        type="button"
                        data-testid={`ontology-issue-${issue.code}`}
                        className="text-left hover:underline"
                        onClick={() => selectBySubject(issue.subject)}
                      >
                        {issueLabel(issue, t)}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </aside>
        <div
          ref={canvasBoxRef}
          className="min-h-0 min-w-0 flex-1 overflow-auto rounded border border-border-button bg-bg-canvas"
          // The dot grid is the canvas's own texture, drawn from the lightest
          // border token so it exists in both themes instead of being a light-mode
          // artifact that turns into grey noise on black.
          style={{
            backgroundImage:
              'radial-gradient(circle, var(--border-button) 1px, transparent 1px)',
            backgroundSize: '16px 16px',
          }}
        >
          <svg
            width={canvasWidth}
            height={canvasHeight}
            role="img"
            aria-label={t('knowledgeCompilation.ontologyCanvas', {
              defaultValue: 'Ontology canvas',
            })}
            className="text-text-primary"
            onPointerMove={(event) => {
              if (rewiring) {
                const point = svgPoint(event);
                setRewiring({ ...rewiring, x: point.x, y: point.y });
                return;
              }
              if (linking) {
                const point = svgPoint(event);
                setLinking({ ...linking, x: point.x, y: point.y });
                return;
              }
              const active = dragging.current;
              if (!active) return;
              const dx = event.clientX - active.x;
              const dy = event.clientY - active.y;
              if (Math.abs(dx) < 2 && Math.abs(dy) < 2) return;
              const node = byId.get(active.id);
              if (!node) return;
              dragging.current = {
                id: active.id,
                x: event.clientX,
                y: event.clientY,
              };
              setPositions((current) => ({
                ...current,
                [active.id]: { x: node.x + dx, y: node.y + dy },
              }));
            }}
            onPointerLeave={() => {
              dragging.current = null;
              setLinking(null);
              setRewiring(null);
            }}
            onPointerUp={() => {
              dragging.current = null;
              // Released over empty canvas: the drag is abandoned rather than
              // landing a relation on whichever entity happens to be first. An
              // arrowhead released there keeps pointing where it already did.
              setLinking(null);
              setRewiring(null);
            }}
          >
            <defs>
              <marker
                id="ontology-arrow"
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" />
              </marker>
            </defs>

            {nodes.flatMap((node) =>
              node.parents.map((parent) => {
                const to = byId.get(parent);
                if (!to) return null;
                // Routed exactly like a property edge: an inheritance line that
                // cuts diagonally across a grid of boxes reads as a different kind
                // of thing, and it is not one.
                const route = routeEdge(
                  nodeBox(to),
                  nodeBox(node),
                  nodes
                    .filter(
                      (other) => other.id !== node.id && other.id !== to.id,
                    )
                    .map((other) => nodeBox(other)),
                );
                return (
                  <path
                    key={`inherit-${parent}-${node.id}`}
                    data-testid={`ontology-inheritance-${parent}-${node.id}`}
                    className="text-text-secondary"
                    d={toPath(route.points)}
                    fill="none"
                    stroke="currentColor"
                    strokeDasharray="4 3"
                    strokeLinejoin="round"
                    markerEnd="url(#ontology-arrow)"
                  />
                );
              }),
            )}

            {linking &&
              byId.get(linking.from) &&
              (() => {
                const box = nodeBox(byId.get(linking.from)!);
                return (
                  <line
                    data-testid="ontology-linking-line"
                    className="text-accent-primary"
                    x1={box.right}
                    y1={box.cy}
                    x2={linking.x}
                    y2={linking.y}
                    stroke="currentColor"
                    strokeWidth={1.5}
                    strokeDasharray="6 4"
                    pointerEvents="none"
                  />
                );
              })()}

            {rewiring &&
              byId.get(rewiring.from) &&
              (() => {
                const box = nodeBox(byId.get(rewiring.from)!);
                return (
                  <line
                    data-testid="ontology-rewiring-line"
                    className="text-accent-primary"
                    x1={box.right}
                    y1={box.cy}
                    x2={rewiring.x}
                    y2={rewiring.y}
                    stroke="currentColor"
                    strokeWidth={1.5}
                    strokeDasharray="6 4"
                    pointerEvents="none"
                  />
                );
              })()}

            {edges.map((edge) => {
              const selected =
                (selection?.kind === 'bundle' && selection.key === edge.key) ||
                (propertyIndex >= 0 &&
                  edge.items.some((item) => item.index === propertyIndex));
              // A self-loop has no two endpoints to join, so both ends sit on the
              // node's right edge and the path bulges out of it; a zero-length line
              // would make `domain == range` invisible on the canvas.
              // A self-loop is squared off too, so the whole picture keeps one
              // geometry: up out of the top edge, around, and back down into it.
              // The right edge belongs to the link handle, and above the box is
              // empty — no rows, no handle.
              const loopHalf = 14;
              const loopTop = edge.fromBox.top - LoopBulge;
              const points = edge.route
                ? edge.route.points
                : [
                    { x: edge.fromBox.cx - loopHalf, y: edge.fromBox.top },
                    { x: edge.fromBox.cx - loopHalf, y: loopTop },
                    { x: edge.fromBox.cx + loopHalf, y: loopTop },
                    { x: edge.fromBox.cx + loopHalf, y: edge.fromBox.top },
                  ];
              const path = toPath(points);
              // One arrowhead is one declaration. A bundle of several properties
              // has no single range to drag, so it keeps the panel as its editor.
              const grip =
                edge.items.length === 1 ? endpointHandle(points) : null;
              // Beside the loop, level with its middle. `labelY` is the position
              // BEFORE the shared lift is applied, so it has to add that back.
              const labelX = edge.route
                ? edge.route.label.x
                : edge.fromBox.cx + loopHalf + 8;
              const labelY = edge.route
                ? edge.route.label.y
                : loopTop + LoopBulge / 2 + 4 + EdgeLabelLift;
              // A routed label sits centred on its segment; a loop's label starts
              // to the right of the loop.
              const labelAnchor = edge.route ? 'middle' : 'start';
              return (
                <g
                  key={`edge-${edge.key}`}
                  data-testid={`ontology-edge-${edge.id}`}
                  className={cn(
                    'cursor-pointer',
                    selected ? 'text-text-primary' : 'text-text-secondary',
                  )}
                  onClick={() =>
                    edge.items.length === 1
                      ? setSelection({
                          kind: 'property',
                          index: edge.items[0].index,
                        })
                      : setSelection({ kind: 'bundle', key: edge.key })
                  }
                >
                  {edge.items.length > 1 && (
                    <title>
                      {edge.items.map((item) => item.label).join(', ')}
                    </title>
                  )}
                  {/* An invisible wide stroke, because a 1px line is a 1px click
                      target: the edge could be seen but not selected, so the panel
                      behind it was unreachable. 12px each side matches what the
                      reference canvas uses. */}
                  <path
                    d={path}
                    fill="none"
                    stroke="transparent"
                    strokeWidth={12}
                    pointerEvents="stroke"
                  />
                  <path
                    d={path}
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={selected ? 2 : 1}
                    // Round joins, so a detour reads as a bent connector rather
                    // than a glyph.
                    strokeLinejoin="round"
                    strokeLinecap="round"
                    markerEnd="url(#ontology-arrow)"
                    // Both ends, when the bundle holds a property in each
                    // direction; the marker is `auto-start-reverse`, so it points
                    // the right way at each end.
                    markerStart={
                      edge.mixed ? 'url(#ontology-arrow)' : undefined
                    }
                  />
                  {/* The label carries a halo of the canvas colour instead of a
                      background rectangle: a rect would need the text measured,
                      and a measured width drifts with the font. paintOrder draws
                      the stroke first, so dense graphs still read and the halo
                      follows the theme on its own. */}
                  <text
                    x={labelX}
                    y={labelY - EdgeLabelLift}
                    fontSize={11}
                    textAnchor={labelAnchor}
                    fill="currentColor"
                    stroke="var(--bg-base)"
                    strokeWidth={3.5}
                    strokeLinejoin="round"
                    paintOrder="stroke"
                  >
                    {edge.text}
                  </text>
                  {/* The arrowhead is a handle: drag it onto another entity and
                      the property's range follows. It only exists while the edge
                      is selected — a grip on every edge would turn the canvas
                      into a row of dots. */}
                  {selected && grip && (
                    <circle
                      data-testid={`ontology-edge-grip-${edge.id}`}
                      className="cursor-crosshair text-accent-primary"
                      cx={grip.x}
                      cy={grip.y}
                      r={6}
                      fill="var(--bg-base)"
                      stroke="currentColor"
                      strokeWidth={2}
                      onPointerDown={(event) =>
                        startRewire(edge.items[0].index, edge.from.id, event)
                      }
                    >
                      <title>
                        {t('knowledgeCompilation.ontologyEdgeGrip', {
                          defaultValue:
                            'Drag the arrowhead to the entity this relation points at. Drop it on the same entity to make it point at itself.',
                        })}
                      </title>
                    </circle>
                  )}
                </g>
              );
            })}

            {nodes.map((node, index) => {
              const selected = entityIndex === index;
              return (
                <g
                  key={`node-${node.id}`}
                  data-testid={`ontology-node-${node.id}`}
                  className="group cursor-move"
                  onPointerDown={(event) => {
                    if (linking || rewiring) return;
                    dragging.current = {
                      id: node.id,
                      x: event.clientX,
                      y: event.clientY,
                    };
                  }}
                  onPointerUp={(event) => {
                    // The arrowhead wins over a new relation: a drag that started
                    // on a grip is not a drag that started on a node anchor.
                    if (rewiring) {
                      event.stopPropagation();
                      finishRewire(node.id);
                      return;
                    }
                    if (!linking) return;
                    event.stopPropagation();
                    finishLink(node.id);
                  }}
                  onClick={() => {
                    if (justRewired.current) {
                      justRewired.current = false;
                      return;
                    }
                    if (justLinked.current) {
                      justLinked.current = false;
                      return;
                    }
                    setSelection({ kind: 'entity', index });
                  }}
                >
                  {/* An HTML card, not positioned <text>: a node is a small
                      document (title, identifier, one row per attribute) and
                      rendering that with <text> meant hand-placing a baseline
                      per row. foreignObject keeps the SVG coordinate system, so
                      the connectors, the drag maths and the hit areas are
                      untouched — only what the node looks like changes. */}
                  <foreignObject
                    x={node.x}
                    y={node.y}
                    width={NodeWidth}
                    height={
                      NodeHeaderHeight +
                      node.attributes.length * AttributeRowHeight
                    }
                  >
                    <div
                      {...{ xmlns: 'http://www.w3.org/1999/xhtml' }}
                      className={cn(
                        'flex h-full flex-col overflow-hidden rounded-lg border bg-bg-component shadow-card',
                        selected
                          ? 'border-text-accent ring-1 ring-text-accent'
                          : 'border-border-button',
                      )}
                    >
                      {/* The title band is tinted and ruled off from the rows
                          below it: on one background the entity name reads as just
                          another line of the card, and the name is the one row a
                          reader scans for. 44px + the rows' 8px padding is the
                          52px the edge geometry expects above the first row. */}
                      <div
                        data-testid={`ontology-node-title-${node.id}`}
                        className={cn(
                          'flex h-11 shrink-0 flex-col justify-center border-b bg-accent-primary/10 px-2 dark:bg-accent-primary/15',
                          selected
                            ? 'border-text-accent'
                            : 'border-border-button',
                        )}
                      >
                        <span className="truncate text-[13px] font-semibold leading-4 text-text-primary">
                          {node.label}
                        </span>
                        <span className="truncate text-[10px] leading-3 text-text-secondary">
                          {node.id}
                        </span>
                      </div>
                      {node.attributes.length > 0 && (
                        <div className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-hidden px-2 py-1">
                          {node.attributes.map((attribute) => (
                            <span
                              key={`${node.id}-${attribute.name}`}
                              className="truncate text-[10px] leading-3 text-text-secondary"
                            >
                              {attribute.name}
                              {attribute.datatype
                                ? `: ${attribute.datatype}`
                                : ''}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  </foreignObject>
                  {/* The link anchor: a dot on the node's right edge that appears
                      on hover. Dragging from it to another entity declares the
                      relation — the gesture the canvas was missing. */}
                  <g
                    data-testid={`ontology-link-anchor-${node.id}`}
                    className="text-accent-primary opacity-40 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                    onPointerDown={(event) => startLink(node.id, event)}
                  >
                    <title>
                      {t('knowledgeCompilation.ontologyLinkAnchor', {
                        defaultValue:
                          'Drag to another entity to declare a relation.',
                      })}
                    </title>
                    <circle
                      cx={node.x + NodeWidth}
                      cy={node.y + NodeHeaderHeight / 2}
                      r={12}
                      fill="transparent"
                    />
                    <circle
                      cx={node.x + NodeWidth}
                      cy={node.y + NodeHeaderHeight / 2}
                      r={5}
                      fill="currentColor"
                      stroke="var(--bg-canvas)"
                      strokeWidth={2}
                    />
                  </g>
                </g>
              );
            })}
          </svg>
        </div>

        {/* A real control, not decoration: drag it, or focus it and use the arrow
            keys, or double-click to go back to the default width. A fixed split
            forces the reader to choose between reading the graph and reading the
            form, and which one they need changes as they work. */}
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label={t('knowledgeCompilation.ontologyResizePanel', {
            defaultValue: 'Resize the panel',
          })}
          title={t('knowledgeCompilation.ontologyResizePanelHint', {
            defaultValue: 'Drag to resize; double-click to reset.',
          })}
          tabIndex={0}
          data-testid="ontology-panel-resizer"
          className="group flex w-2 shrink-0 cursor-col-resize items-center justify-center rounded hover:bg-bg-card focus-visible:outline focus-visible:outline-1"
          onPointerDown={beginResize}
          onPointerMove={applyResize}
          onPointerUp={endResize}
          onPointerCancel={endResize}
          onMouseDown={beginResize}
          onMouseMove={applyResize}
          onMouseUp={endResize}
          onDoubleClick={() => setPanelWidth(DefaultPanelWidth)}
          onKeyDown={(event) => {
            if (event.key === 'ArrowLeft') {
              // Left widens: the divider moves left, so the panel grows.
              event.preventDefault();
              nudgePanel(PanelResizeStep);
            } else if (event.key === 'ArrowRight') {
              event.preventDefault();
              nudgePanel(-PanelResizeStep);
            }
          }}
        >
          <span className="h-8 w-0.5 rounded bg-border-button group-hover:bg-text-secondary" />
        </div>

        <aside
          data-testid="ontology-detail-panel"
          style={{ width: `${panelWidth}px` }}
          className="flex max-h-full min-h-0 shrink-0 flex-col gap-2 overflow-y-auto rounded border border-border-button p-3 text-xs"
        >
          {!selection && (
            <p className="text-text-secondary">
              {t('knowledgeCompilation.ontologySelectHint', {
                defaultValue: 'Select an entity or a property on the canvas.',
              })}
            </p>
          )}

          {selectedEntity && (
            <>
              <strong>
                {t('knowledgeCompilation.ontologyClass', {
                  defaultValue: 'Entity',
                })}
              </strong>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldType')}
                <Input
                  value={String(selectedEntity.type ?? '')}
                  data-testid="ontology-entity-type"
                  onChange={(event) =>
                    editClass(entityIndex, { type: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldLabel')}
                <Input
                  value={String(selectedEntity.label ?? '')}
                  onChange={(event) =>
                    editClass(entityIndex, { label: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.description')}
                <Textarea
                  rows={2}
                  value={String(selectedEntity.description ?? '')}
                  data-testid="ontology-entity-description"
                  onChange={(event) =>
                    editClass(entityIndex, { description: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldParent')}
                <SelectWithSearch
                  value={splitTypeList(selectedEntity.parent)[0] ?? ''}
                  options={[{ label: '—', value: '' }, ...entityOptions]}
                  onChange={(value) =>
                    editClass(entityIndex, { parent: value })
                  }
                />
              </label>
              {/* The other names this entity is known by. They reach the prompt as
                  a hint and are resolved back to `type` on the way in, so a model
                  answering "company" for a declared `organization` does not create
                  a second spelling of the same entity. */}
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldAliases', {
                  defaultValue: 'Aliases (| separated)',
                })}
                <Input
                  value={valueListText(selectedEntity.aliases)}
                  data-testid="ontology-entity-aliases"
                  onChange={(event) =>
                    editClass(entityIndex, { aliases: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldRule')}
                <Textarea
                  rows={3}
                  value={String(selectedEntity.rule ?? '')}
                  onChange={(event) =>
                    editClass(entityIndex, { rule: event.target.value })
                  }
                />
              </label>
              {/* An action's contract is part of its declaration, so it is edited
                  here rather than on an instance: which of the six fields this
                  model requires is a property of the model, while the values
                  belong to each action row. */}
              {String(selectedEntity.type ?? '').trim() === 'action' && (
                <div
                  className="flex flex-col gap-1 rounded border border-border-button p-2"
                  data-testid="ontology-entity-contract"
                >
                  <span className="font-medium">
                    {t('knowledgeCompilation.ontologyContractFields', {
                      defaultValue: 'Required contract fields',
                    })}
                  </span>
                  {ACTION_CONTRACT_FIELD_ORDER.map((name) => {
                    // The declaration is read through the same normalisation the
                    // server uses, so the boxes cannot show a field as dropped
                    // that the extractor is still asked for.
                    const declared = declaredContractFields(
                      selectedEntity.contract,
                    );
                    const checked = declared.includes(name);
                    // Authorization decides whether the action may ever run
                    // unattended, and a field the extractor was never asked for
                    // cannot be stated — so it is not narrowable away.
                    const locked = name === 'authorization';
                    return (
                      <label key={name} className="flex items-center gap-2">
                        <Checkbox
                          checked={checked}
                          disabled={locked}
                          data-testid={`ontology-contract-field-${name}`}
                          onCheckedChange={(next) => {
                            const current = declaredContractFields(
                              selectedEntity.contract,
                            );
                            const nextList =
                              next === true
                                ? [...new Set([...current, name])]
                                : current.filter((item) => item !== name);
                            editClass(entityIndex, {
                              contract: nextList.join('|'),
                            });
                          }}
                        />
                        <span>{contractFieldLabel(name, t)}</span>
                      </label>
                    );
                  })}
                  <p className="text-text-disabled">
                    {t('knowledgeCompilation.ontologyContractFieldsHint', {
                      defaultValue:
                        'Nothing ticked means no requirement, which reads as all six. Authorization is always asked: it decides whether the action may run. The entity rule should also state that an action needs an actor and an object.',
                    })}
                  </p>
                </div>
              )}
              {/* The entity's own properties, listed where the reader already is.
                  A property with no domain was findable nowhere, which is why
                  "which entity does this belong to" had no answer. */}
              <div
                className="flex flex-col gap-1"
                data-testid="ontology-own-properties"
              >
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyOwnProperties', {
                    defaultValue: 'Properties of this entity',
                  })}
                </span>
                {ownProperties.length === 0 ? (
                  <span className="text-text-secondary">
                    {t('knowledgeCompilation.ontologyNoOwnProperties', {
                      defaultValue: 'None yet.',
                    })}
                  </span>
                ) : (
                  <ul className="flex flex-col gap-0.5">
                    {ownProperties.map((own) => (
                      <li key={`${own.index}-${own.label}`}>
                        <button
                          type="button"
                          className="w-full truncate text-left hover:text-text-primary"
                          onClick={() =>
                            setSelection({ kind: 'property', index: own.index })
                          }
                        >
                          {own.label}
                          {own.target ? (
                            <span className="opacity-60">{` → ${own.target}`}</span>
                          ) : null}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  data-testid="ontology-add-own-attribute"
                  onClick={addOwnAttribute}
                >
                  {t('knowledgeCompilation.ontologyAddOwnAttribute', {
                    defaultValue: '+ Attribute',
                  })}
                </Button>
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyRelationsViaRail', {
                    defaultValue:
                      'Drag from an entity’s edge dot to another entity, or add one in the Relations tab.',
                  })}
                </span>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  write(removeEntity(draft, entityIndex));
                  setSelection(null);
                }}
              >
                {t('knowledgeCompilation.ontologyDeleteClass', {
                  defaultValue: 'Delete entity',
                })}
              </Button>
            </>
          )}

          {selectedBundle && (
            <>
              <strong>
                {t('knowledgeCompilation.ontologyBundleHeader', {
                  count: selectedBundle.items.length,
                  defaultValue: '{{count}} properties share this edge',
                })}
              </strong>
              <ul className="flex flex-col gap-0.5">
                {selectedBundle.items.map((item) => (
                  <li key={item.index}>
                    <button
                      type="button"
                      className="w-full truncate text-left hover:text-text-primary"
                      onClick={() =>
                        setSelection({ kind: 'property', index: item.index })
                      }
                    >
                      {item.label}
                    </button>
                  </li>
                ))}
              </ul>
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyBundleHint', {
                  defaultValue: 'Pick one to edit it.',
                })}
              </span>
            </>
          )}

          {selectedProperty && (
            <>
              <strong>
                {t('knowledgeCompilation.ontologyProperty', {
                  defaultValue: 'Property',
                })}
              </strong>
              <span
                className="text-text-secondary"
                data-testid="ontology-property-owner"
              >
                {t('knowledgeCompilation.ontologyBelongsTo', {
                  defaultValue: 'Belongs to',
                })}
                {': '}
                {selectedPropertyDomain ? (
                  selectedPropertyRange ? (
                    `${selectedPropertyDomain} → ${selectedPropertyRange}`
                  ) : (
                    selectedPropertyDomain
                  )
                ) : (
                  <span className="text-[#F0A020]">
                    {t('knowledgeCompilation.ontologyNoDomain', {
                      defaultValue:
                        'no entity yet — the canvas draws nothing for it',
                    })}
                  </span>
                )}
              </span>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldType')}
                <Input
                  value={String(selectedProperty.type ?? '')}
                  data-testid="ontology-property-type"
                  onChange={(event) =>
                    editProperty(propertyIndex, { type: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldAliases', {
                  defaultValue: 'Aliases (| separated)',
                })}
                <Input
                  value={valueListText(selectedProperty.aliases)}
                  data-testid="ontology-property-aliases"
                  onChange={(event) =>
                    editProperty(propertyIndex, { aliases: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldKind')}
                <SelectWithSearch
                  value={selectedKind}
                  options={[
                    { label: OntologyKind.Object, value: OntologyKind.Object },
                    {
                      label: OntologyKind.Datatype,
                      value: OntologyKind.Datatype,
                    },
                  ]}
                  onChange={(value) =>
                    editProperty(propertyIndex, { kind: value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldDomain')}
                <SelectWithSearch
                  value={splitTypeList(selectedProperty.domain)[0] ?? ''}
                  options={[{ label: '—', value: '' }, ...entityOptions]}
                  onChange={(value) =>
                    editProperty(propertyIndex, { domain: value })
                  }
                />
              </label>
              {selectedKind === OntologyKind.Object && (
                <label className="flex flex-col gap-1">
                  {t('knowledgeCompilation.fieldRange')}
                  <SelectWithSearch
                    value={splitTypeList(selectedProperty.range)[0] ?? ''}
                    options={[{ label: '—', value: '' }, ...entityOptions]}
                    onChange={(value) =>
                      editProperty(propertyIndex, { range: value })
                    }
                  />
                </label>
              )}
              {selectedKind === OntologyKind.Datatype && (
                <>
                  <label className="flex flex-col gap-1">
                    {t('knowledgeCompilation.fieldDatatype')}
                    <SelectWithSearch
                      value={String(selectedProperty.datatype ?? '')}
                      options={OntologyDatatypeKinds.map((kind) => ({
                        label: kind,
                        value: kind,
                      }))}
                      onChange={(value) =>
                        editProperty(propertyIndex, { datatype: value })
                      }
                    />
                  </label>
                  {/* A closed value set: stated to the model so it cannot invent a
                      category, and enforced on the stored value so a stray one
                      never reaches a filter. */}
                  <label className="flex flex-col gap-1">
                    {t('knowledgeCompilation.fieldEnum', {
                      defaultValue: 'Allowed values (| separated)',
                    })}
                    <Input
                      value={valueListText(selectedProperty.enum)}
                      data-testid="ontology-property-enum"
                      onChange={(event) =>
                        editProperty(propertyIndex, {
                          enum: event.target.value,
                        })
                      }
                    />
                  </label>
                  {/* Acceptance only: this is never rendered into the prompt,
                      because a model told an attribute is mandatory supplies one
                      instead of leaving a gap. It is judged on the stored rows and
                      reported as `missing` on the entity's attribute list. */}
                  <label className="flex flex-col gap-1">
                    {t('knowledgeCompilation.fieldRequired', {
                      defaultValue: 'Required on every instance',
                    })}
                    <SelectWithSearch
                      value={
                        valueListTruthy(selectedProperty.required) ? 'true' : ''
                      }
                      options={[
                        { label: '—', value: '' },
                        { label: 'true', value: 'true' },
                      ]}
                      onChange={(value) =>
                        editProperty(propertyIndex, { required: value })
                      }
                    />
                  </label>
                </>
              )}
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.description')}
                <Textarea
                  rows={2}
                  value={String(selectedProperty.description ?? '')}
                  data-testid="ontology-property-description"
                  onChange={(event) =>
                    editProperty(propertyIndex, {
                      description: event.target.value,
                    })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldRule')}
                <Textarea
                  rows={3}
                  value={String(selectedProperty.rule ?? '')}
                  onChange={(event) =>
                    editProperty(propertyIndex, { rule: event.target.value })
                  }
                />
              </label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  write(removeProperty(draft, propertyIndex));
                  setSelection(null);
                }}
              >
                {t('knowledgeCompilation.ontologyDeleteProperty', {
                  defaultValue: 'Delete property',
                })}
              </Button>
            </>
          )}

          {selectedIssues.length > 0 && (
            <ul className="flex flex-col gap-1 text-[#F0A020]">
              {selectedIssues.map((issue, at) => (
                <li key={`${issue.code}-${at}`}>{issueLabel(issue, t)}</li>
              ))}
            </ul>
          )}
        </aside>
      </div>
    </div>
  );
}

export default OntologyCanvasEditor;
