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
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '../schema';
import {
  OntologyDatatypeKinds,
  OntologyKind,
  addClass,
  addProperty,
  canonicalProperties,
  draftFromSections,
  effectiveKind,
  removeClass,
  removeProperty,
  renameClass,
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
const DefaultPanelWidth = 432;
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
 * The rectangle a class occupies. Its height grows with its attribute rows, so the
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
 * it; anchoring on the border is what makes the arrow point AT the class.
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
    (point, at) => at === 0 || point.x !== points[at - 1].x || point.y !== points[at - 1].y,
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
    blockers.filter((box) => box.cy >= fromCentre.y).length >= blockers.length / 2
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
 * Placed on a grid, in declaration order: adding a class never moves the others.
 */
export const nodePosition = (index: number, columns: number) => ({
  x: CanvasPadding + (index % columns) * CellWidth,
  y: CanvasPadding + Math.floor(index / columns) * CellHeight,
});

export const gridColumns = (count: number) =>
  Math.max(
    1,
    Math.min(
      Math.max(count, 1),
      Math.ceil(Math.sqrt(Math.max(count, 1))) + 1,
    ),
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

interface OntologyCanvasEditorProps {
  form: UseFormReturn<FormSchemaType>;
  templateIndex: number;
}

type Selection =
  | { kind: 'class'; index: number }
  | { kind: 'property'; index: number }
  // A pair of classes can carry several properties. They are drawn as ONE edge:
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
 * It is the ontology counterpart of the card grid: classes are nodes, object
 * properties are the arrows between them, and a class's datatype properties are
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
  // Click the handle, then click the target class: linking by two clicks instead
  // of a drag is the same two decisions with no pointer capture to get wrong, and
  // it leaves the canvas usable on a trackpad.
  const [linkingFrom, setLinkingFrom] = useState<string | null>(null);
  // A provisional line follows the pointer and Esc cancels. Without either, a
  // click on the handle only changes one faint circle's opacity and reads as
  // broken — which is exactly how it was reported.
  const [linkPointer, setLinkPointer] = useState<{
    x: number;
    y: number;
  } | null>(null);
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
    setPanelWidth((current) =>
      Math.max(MinPanelWidth, current + delta),
    );

  useEffect(() => {
    if (!linkingFrom) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setLinkingFrom(null);
        setLinkPointer(null);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [linkingFrom]);

  const properties = useMemo(
    () => canonicalProperties(draft.properties),
    [draft],
  );
  const classSet = useMemo(
    () =>
      new Set(
        draft.classes
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
        write(renameClass(draft, index, patch.type));
        return;
      }
      write({ ...draft, classes: updateField(draft.classes, index, patch) });
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

  const columns = gridColumns(draft.classes.length);
  const nodes = draft.classes.map((field, index) => {
    const id = String(field.type ?? '').trim();
    const at = positions[id] ?? nodePosition(index, columns);
    return {
      id,
      label: String(field.label ?? '').trim() || id,
      parents: splitTypeList(field.parent).filter((parent) => classSet.has(parent)),
      attributes: properties
        .filter(
          (property) =>
            effectiveKind(property, classSet) === OntologyKind.Datatype &&
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
      if (effectiveKind(property, classSet) !== OntologyKind.Object) return;
      const fromId = splitTypeList(property.domain)[0];
      const toId = splitTypeList(property.range)[0];
      const from = byId.get(fromId);
      const to = byId.get(toId);
      if (!from || !to) return;
      // Keyed by the UNORDERED pair: `a -> b` and `b -> a` between the same two
      // classes are one visual edge. Drawing them as two opposing curves is part
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
      text:
        entry.items.length === 1 ? entry.items[0].label : `×${entry.items.length}`,
      mixed:
        entry.items.some((item) => item.forward) &&
        entry.items.some((item) => !item.forward),
    }));
  })();

  const rows = Math.ceil(Math.max(draft.classes.length, 1) / columns);
  const contentWidth = CanvasPadding * 2 + Math.max(columns, 1) * CellWidth;
  const contentHeight = CanvasPadding * 2 + rows * CellHeight;

  // The canvas is as big as the space it was given, and no smaller than its
  // content: the grid still decides where the classes ARE (positions come from
  // `nodePosition`, not from this box), so measurement only ever adds empty
  // room to drag into — it can never move a class.
  const canvasBoxRef = useRef<HTMLDivElement>(null);
  const canvasBox = useContainerDimensions(canvasBoxRef);
  const canvasWidth = Math.max(contentWidth, canvasBox.width);
  const canvasHeight = Math.max(contentHeight, canvasBox.height);

  const classIndex = selection?.kind === 'class' ? selection.index : -1;
  const propertyIndex = selection?.kind === 'property' ? selection.index : -1;
  const selectedClass = draft.classes[classIndex];
  const selectedProperty = properties[propertyIndex];
  const selectedKind =
    selectedProperty === undefined
      ? ''
      : String(selectedProperty.kind ?? '').trim() ||
        effectiveKind(selectedProperty, classSet);
  const selectedIssues = issues.filter(
    (issue) =>
      issue.subject !== '' &&
      issue.subject ===
        String(
          (selectedClass ?? selectedProperty)?.type ?? '',
        ).trim(),
  );

  // Which properties belong to the selected class. A property created with no
  // domain was findable nowhere — this list is one of the two answers to "which
  // entity does this belong to"; the attribute rows drawn on the node are the
  // other, and both come from the same declaration.
  const selectedClassType = String(selectedClass?.type ?? '').trim();
  const ownProperties = selectedClass
    ? properties
        .map((property, index) => ({ property, index }))
        .filter(({ property }) =>
          splitTypeList(property.domain).includes(selectedClassType),
        )
        .map(({ property, index }) => ({
          index,
          label:
            String(property.label ?? '').trim() ||
            String(property.type ?? '').trim(),
          target:
            effectiveKind(property, classSet) === OntologyKind.Datatype
              ? ''
              : (splitTypeList(property.range)[0] ?? ''),
        }))
    : [];

  // A finding names a class or a property, so it can take the reader there. This is
  // the ONLY way to reach a property with no domain: it draws no edge, so it cannot
  // be clicked on the canvas, and it is in no class's list either.
  const selectBySubject = (subject: string) => {
    const classAt = draft.classes.findIndex(
      (item) => String(item.type ?? '').trim() === subject,
    );
    if (classAt >= 0) {
      setSelection({ kind: 'class', index: classAt });
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

  const selectedPropertyDomain = splitTypeList(selectedProperty?.domain)[0] ?? '';
  const selectedPropertyRange = splitTypeList(selectedProperty?.range)[0] ?? '';

  // Anchored on creation: the domain is the class the reader is looking at, so the
  // new attribute shows up on that node immediately and can never be an orphan.
  // That is also the state the reference implementation's form enforces — its
  // domain and range are both `required`.
  const addOwnAttribute = () => {
    const next = addProperty(draft, OntologyKind.Datatype);
    const created = next.properties.length - 1;
    write({
      ...next,
      properties: updateField(next.properties, created, {
        domain: selectedClassType,
      }),
    });
    setSelection({ kind: 'property', index: created });
  };

  const classOptions = [...classSet]
    .sort()
    .map((name) => ({ label: name, value: name }));

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            const next = addClass(draft);
            write(next);
            setSelection({ kind: 'class', index: next.classes.length - 1 });
          }}
        >
          {t('knowledgeCompilation.ontologyAddClass', {
            defaultValue: '+ Class',
          })}
        </Button>
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
        {linkingFrom && (
          <div
            data-testid="ontology-linking-banner"
            className="rounded border border-[#4CACFF] bg-bg-card px-3 py-1 text-xs text-text-primary"
          >
            {t('knowledgeCompilation.ontologyLinkingHint', {
              from: linkingFrom,
              defaultValue:
                'Click a class to connect it to {{from}}, or click empty canvas to add a new class joined to it (Esc cancels).',
            })}
          </div>
        )}
        <span className="text-xs text-text-secondary">
          {t('knowledgeCompilation.ontologyDragHint', {
            defaultValue:
              'Drag a class to move it. Positions are not stored in the template.',
          })}
        </span>
      </div>

      {issues.length > 0 && (
        <ul className="rounded border border-[#F0A020] px-3 py-2 text-xs text-[#F0A020]">
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

      <div className="flex min-h-0 flex-1 gap-3">
        <div
          ref={canvasBoxRef}
          className="min-h-0 min-w-0 flex-1 overflow-auto rounded border border-border-button bg-bg-base"
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
              if (linkingFrom) {
                // Client coordinates are viewport-relative, so this stays right
                // however the canvas box is scrolled.
                const rect = event.currentTarget.getBoundingClientRect();
                setLinkPointer({
                  x: event.clientX - rect.left,
                  y: event.clientY - rect.top,
                });
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
            }}
            onClick={(event) => {
              if (!linkingFrom) return;
              // Empty canvas means "grow the graph from here": a new class lands
              // where the reader clicked, joined to the source by a property. That
              // is the reading the "+" invites, and it saves the
              // create-a-class-then-connect-it round trip. A click on a NODE stops
              // propagation, so the two outcomes never both fire.
              const rect = event.currentTarget.getBoundingClientRect();
              const next = addProperty(addClass(draft), OntologyKind.Object);
              const createdIndex = next.classes.length - 1;
              const createdProperty = next.properties.length - 1;
              const created = String(
                next.classes[createdIndex]?.type ?? '',
              ).trim();
              write({
                ...next,
                properties: updateField(next.properties, createdProperty, {
                  type: uniquePropertyName(next, `${linkingFrom}_${created}`),
                  domain: linkingFrom,
                  range: created,
                }),
              });
              setPositions((current) => ({
                ...current,
                [created]: {
                  x: event.clientX - rect.left - NodeWidth / 2,
                  y: event.clientY - rect.top - NodeHeaderHeight / 2,
                },
              }));
              setLinkingFrom(null);
              setLinkPointer(null);
              setSelection({ kind: 'class', index: createdIndex });
            }}
            onPointerUp={() => {
              dragging.current = null;
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

            {linkingFrom && linkPointer && byId.get(linkingFrom) && (
              // The provisional edge: this is what makes the handle's click
              // visible, and it points at whatever the reader is about to choose.
              <line
                data-testid="ontology-linking-line"
                className="text-[#4CACFF]"
                x1={borderPoint(nodeBox(byId.get(linkingFrom)!), linkPointer).x}
                y1={borderPoint(nodeBox(byId.get(linkingFrom)!), linkPointer).y}
                x2={linkPointer.x}
                y2={linkPointer.y}
                stroke="currentColor"
                strokeDasharray="6 4"
                pointerEvents="none"
              />
            )}

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
              const path = edge.route
                ? toPath(edge.route.points)
                : toPath([
                    { x: edge.fromBox.cx - loopHalf, y: edge.fromBox.top },
                    { x: edge.fromBox.cx - loopHalf, y: loopTop },
                    { x: edge.fromBox.cx + loopHalf, y: loopTop },
                    { x: edge.fromBox.cx + loopHalf, y: edge.fromBox.top },
                  ]);
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
                  <text
                    x={labelX}
                    y={labelY - EdgeLabelLift}
                    fontSize={11}
                    textAnchor={labelAnchor}
                    fill="currentColor"
                  >
                    {edge.text}
                  </text>
                </g>
              );
            })}

            {nodes.map((node, index) => {
              const selected = classIndex === index;
              return (
                <g
                  key={`node-${node.id}`}
                  data-testid={`ontology-node-${node.id}`}
                  className="cursor-move"
                  onPointerDown={(event) => {
                    dragging.current = {
                      id: node.id,
                      x: event.clientX,
                      y: event.clientY,
                    };
                  }}
                  onClick={(event) => {
                    if (linkingFrom === node.id) {
                      event.stopPropagation();
                      setLinkingFrom(null);
                      setLinkPointer(null);
                      return;
                    }
                    if (linkingFrom) {
                      event.stopPropagation();
                      // Two clicks, one property: the domain is the class the
                      // handle was taken from, the range the one clicked. The
                      // declaration is written straight into the template's own
                      // field shape by the model layer.
                      const next = addProperty(draft, OntologyKind.Object);
                      const created = next.properties.length - 1;
                      write({
                        ...next,
                        properties: updateField(next.properties, created, {
                          type: uniquePropertyName(
                            draft,
                            `${linkingFrom}_${node.id}`,
                          ),
                          domain: linkingFrom,
                          range: node.id,
                        }),
                      });
                      setLinkingFrom(null);
                      setLinkPointer(null);
                      setSelection({ kind: 'property', index: created });
                      return;
                    }
                    setSelection({ kind: 'class', index });
                  }}
                >
                  <rect
                    x={node.x}
                    y={node.y}
                    width={NodeWidth}
                    height={
                      NodeHeaderHeight + node.attributes.length * AttributeRowHeight
                    }
                    rx={8}
                    fill="currentColor"
                    fillOpacity={selected ? 0.08 : 0.02}
                    stroke="currentColor"
                    strokeWidth={selected ? 2 : 1}
                  />
                  <text
                    x={node.x + 10}
                    y={node.y + 20}
                    fontSize={13}
                    fontWeight={600}
                    fill="currentColor"
                  >
                    {node.label}
                  </text>
                  <text
                    x={node.x + 10}
                    y={node.y + 36}
                    fontSize={10}
                    fill="currentColor"
                    opacity={0.7}
                  >
                    {node.id}
                  </text>
                  {/* The link handle: the one gesture that makes this a canvas
                      rather than a form. */}
                  <circle
                    data-testid={`ontology-link-handle-${node.id}`}
                    cx={node.x + NodeWidth + 10}
                    cy={node.y + NodeHeaderHeight / 2}
                    r={10}
                    fill="currentColor"
                    fillOpacity={linkingFrom === node.id ? 0.3 : 0.08}
                    stroke="currentColor"
                    onClick={(event) => {
                      event.stopPropagation();
                      setLinkPointer(null);
                      setLinkingFrom((current) =>
                        current === node.id ? null : node.id,
                      );
                    }}
                  >
                    <title>
                      {t('knowledgeCompilation.ontologyLinkHandle', {
                        defaultValue:
                          'Link: click, then click a class — or click empty canvas to add a new class joined to this one.',
                      })}
                    </title>
                  </circle>
                  <text
                    x={node.x + NodeWidth + 10}
                    y={node.y + NodeHeaderHeight / 2 + 3}
                    fontSize={10}
                    textAnchor="middle"
                    fill="currentColor"
                    pointerEvents="none"
                  >
                    +
                  </text>
                  {node.attributes.map((attribute, at) => (
                    <text
                      key={`${node.id}-${attribute.name}`}
                      x={node.x + 12}
                      y={node.y + 56 + at * AttributeRowHeight}
                      fontSize={10}
                      fill="currentColor"
                      opacity={0.85}
                    >
                      {attribute.name}
                      {attribute.datatype ? `: ${attribute.datatype}` : ''}
                    </text>
                  ))}
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
                defaultValue: 'Select a class or a property on the canvas.',
              })}
            </p>
          )}

          {selectedClass && (
            <>
              <strong>
                {t('knowledgeCompilation.ontologyClass', {
                  defaultValue: 'Class',
                })}
              </strong>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldType')}
                <Input
                  value={String(selectedClass.type ?? '')}
                  data-testid="ontology-class-type"
                  onChange={(event) =>
                    editClass(classIndex, { type: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldLabel')}
                <Input
                  value={String(selectedClass.label ?? '')}
                  onChange={(event) =>
                    editClass(classIndex, { label: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.description')}
                <Textarea
                  rows={2}
                  value={String(selectedClass.description ?? '')}
                  data-testid="ontology-class-description"
                  onChange={(event) =>
                    editClass(classIndex, { description: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldParent')}
                <SelectWithSearch
                  value={splitTypeList(selectedClass.parent)[0] ?? ''}
                  options={[{ label: '—', value: '' }, ...classOptions]}
                  onChange={(value) => editClass(classIndex, { parent: value })}
                />
              </label>
              {/* The other names this class is known by. They reach the prompt as
                  a hint and are resolved back to `type` on the way in, so a model
                  answering "company" for a declared `organization` does not create
                  a second spelling of the same class. */}
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldAliases', {
                  defaultValue: 'Aliases (| separated)',
                })}
                <Input
                  value={valueListText(selectedClass.aliases)}
                  data-testid="ontology-class-aliases"
                  onChange={(event) =>
                    editClass(classIndex, { aliases: event.target.value })
                  }
                />
              </label>
              <label className="flex flex-col gap-1">
                {t('knowledgeCompilation.fieldRule')}
                <Textarea
                  rows={3}
                  value={String(selectedClass.rule ?? '')}
                  onChange={(event) =>
                    editClass(classIndex, { rule: event.target.value })
                  }
                />
              </label>
              {/* The class's own properties, listed where the reader already is.
                  A property with no domain was findable nowhere, which is why
                  "which entity does this belong to" had no answer. */}
              <div
                className="flex flex-col gap-1"
                data-testid="ontology-own-properties"
              >
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyOwnProperties', {
                    defaultValue: 'Properties of this class',
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
                  {t('knowledgeCompilation.ontologyRelationsViaHandle', {
                    defaultValue: 'Relations start at the + handle on the node.',
                  })}
                </span>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  write(removeClass(draft, classIndex));
                  setSelection(null);
                }}
              >
                {t('knowledgeCompilation.ontologyDeleteClass', {
                  defaultValue: 'Delete class',
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
                        'no class yet — the canvas draws nothing for it',
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
                  options={[{ label: '—', value: '' }, ...classOptions]}
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
                    options={[{ label: '—', value: '' }, ...classOptions]}
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
                        editProperty(propertyIndex, { enum: event.target.value })
                      }
                    />
                  </label>
                  {/* Acceptance only: this is never rendered into the prompt,
                      because a model told an attribute is mandatory supplies one
                      instead of leaving a gap. It is judged on the stored rows and
                      reported as `missing` on the class's attribute list. */}
                  <label className="flex flex-col gap-1">
                    {t('knowledgeCompilation.fieldRequired', {
                      defaultValue: 'Required on every instance',
                    })}
                    <SelectWithSearch
                      value={
                        valueListTruthy(selectedProperty.required)
                          ? 'true'
                          : ''
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
