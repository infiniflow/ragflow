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

import { useContainerDimensions } from '@/components/artifact-force-graph/use-container-dimensions';
import type { IOntologyGraph } from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph2D, { type ForceGraphMethods } from 'react-force-graph-2d';
import type {
  OntologyDetailPanelState,
  OntologyDetailProperty,
} from './detail-panel';
import {
  buildOntologyGraphData,
  computeOntologyLayout,
  linkEndpointId,
  type OntologyGraphLink,
  type OntologyGraphNode,
} from './graph-data';
import {
  PitfallCategoryOrder,
  pitfallCategoryLabel,
  pitfallLabel,
} from './pitfall-labels';

const ClassColor = '#4CACFF';
const UndeclaredClassColor = '#F0A020';
const EdgeColor = '#9CA3AF';
/** rdfs:subClassOf is drawn in its own colour so it never reads as a property. */
const InheritanceColor = '#8B5CF6';
const HighlightColor = '#00BEB4';
const DimmedAlpha = 0.15;
const HighlightAlpha = 1;

const withAlpha = (hex: string, alpha: number) => {
  const value = hex.replace('#', '');
  const r = parseInt(value.slice(0, 2), 16);
  const g = parseInt(value.slice(2, 4), 16);
  const b = parseInt(value.slice(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
};

type OntologyNodeObject = OntologyGraphNode;

/** One label's place and size on the canvas, for the declutter pass. */
interface LabelBox {
  x: number;
  y: number;
  width: number;
  height: number;
  /** Radians; the label is drawn rotated so it follows its edge. */
  angle: number;
  /**
   * Who owns this box. A painter must ignore its own claim when deciding
   * whether it may draw, or the node that reserved the space would find itself
   * colliding with its own reservation and print nothing.
   */
  owner?: string;
}

interface LinkLabel extends LabelBox {
  text: string;
  fontSize: number;
  highlighted: boolean;
}

/** A class name placed on one of the four sides of its disc. */
interface NodeLabelPlacement {
  x: number;
  y: number;
  align: CanvasTextAlign;
  baseline: CanvasTextBaseline;
  text: string;
  count: string | null;
  box: LabelBox;
}

/**
 * Where a class name may go, tried in this order. A name that cannot fit below
 * its disc is moved around it rather than dropped: a node without a name is not
 * a node any more, which is why node labels get to try sides while edge labels
 * only get one shot.
 */
const NodeLabelSides = (
  radius: number,
): {
  x: number;
  y: number;
  align: CanvasTextAlign;
  baseline: CanvasTextBaseline;
}[] => [
  { x: 0, y: radius + 2, align: 'center', baseline: 'top' },
  { x: 0, y: -radius - 2, align: 'center', baseline: 'bottom' },
  { x: radius + 6, y: 0, align: 'left', baseline: 'middle' },
  { x: -radius - 6, y: 0, align: 'right', baseline: 'middle' },
];

/**
 * labelsOverlap compares two rotated label boxes by their axis-aligned bounds.
 *
 * Exact polygon intersection is not worth it here: a label is a thin strip, so
 * the bound is both cheaper and stricter — which is what a declutter pass wants,
 * because two labels that merely graze each other are still unreadable.
 */
const labelsOverlap = (a: LabelBox, b: LabelBox): boolean => {
  const halfSize = (box: LabelBox) => {
    const cos = Math.abs(Math.cos(box.angle));
    const sin = Math.abs(Math.sin(box.angle));
    return {
      w: (box.width * cos + box.height * sin) / 2,
      h: (box.width * sin + box.height * cos) / 2,
    };
  };
  const ha = halfSize(a);
  const hb = halfSize(b);
  return Math.abs(a.x - b.x) < ha.w + hb.w && Math.abs(a.y - b.y) < ha.h + hb.h;
};

interface OntologyModelGraphProps {
  graph?: IOntologyGraph;
  /** Class name to emphasize, controlled by the host page. */
  highlightClass?: string | null;
  className?: string;
  /**
   * Scope for the drill-down. Without it the graph still renders — every piece
   * of the skeleton and every count is already in `graph` — but the detail panel
   * does not offer to list the underlying rows.
   */
  datasetId?: string;
  /** The document, on a document page: the drill-down must match the graph's scope. */
  documentId?: string;
  templateId?: string;
  /**
   * Publishes the selected class so the HOST renders the detail column.
   *
   * The panel is deliberately not drawn inside this component: the canvas sits
   * in the artifact column, and a panel rendered there is squashed (or covered)
   * as soon as the column divider moves — the divider only knows about the
   * page's own panels. Publishing it lets the page put it in a real column.
   */
  onClassDetailChange?: (state: OntologyDetailPanelState | null) => void;
}

/**
 * OntologyModelGraph renders the class-level ontology: classes are nodes and
 * properties are directed domain -> range edges, with each element annotated by
 * how many instances the compiled scope holds.
 *
 * The skeleton comes from the template config while the counts come from the
 * index, so a declared class with zero instances is still drawn — the view
 * shows the ontology the template declares, not only the part the data reached.
 */
function OntologyModelGraph({
  graph,
  highlightClass,
  className,
  datasetId,
  documentId,
  templateId,
  onClassDetailChange,
}: OntologyModelGraphProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<OntologyGraphNode> | undefined>(
    undefined,
  );
  const hasFittedRef = useRef(false);
  const dimensions = useContainerDimensions(containerRef);
  const hasDimensions = dimensions.width > 0 && dimensions.height > 0;
  // The ring layout is a function of the canvas size. Quantizing it keeps a
  // resize drag from rebuilding the layout — and republishing the detail panel —
  // on every animation frame.
  const layoutWidth = Math.round(dimensions.width / 40) * 40;
  const layoutHeight = Math.round(dimensions.height / 40) * 40;

  const [hovered, setHovered] = useState<OntologyGraphNode | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  // Edges are first-class: hovering one names it, clicking one opens the panel
  // of the class that declares those properties (the domain), which is where its
  // assertions can be listed.
  const [hoveredLink, setHoveredLink] = useState<OntologyGraphLink | null>(null);
  const [selectedLink, setSelectedLink] = useState<OntologyGraphLink | null>(
    null,
  );
  const [query, setQuery] = useState('');
  // Inheritance is a second, independent layer: a viewer can read the property
  // graph without it, so it can be switched off.
  const [showInheritance, setShowInheritance] = useState(true);
  // Vocabulary drift is hidden by default: the classes and property edges the
  // template does not declare are what turn the canvas into a pile on a document
  // whose model invented its own vocabulary. One toggle brings them back, so the
  // drift is a click away rather than the default view — the counts and the
  // pitfalls panel still report it either way.
  const [showUndeclared, setShowUndeclared] = useState(false);
  // "Only the focused class's edges": the escape hatch for a dense template,
  // where the full property graph is a mesh no force layout can untangle.
  // Inheritance stays, because it is the backbone that gives the classes their
  // place. Applied in the painter rather than to the graph data, so toggling or
  // changing the selection never reheats the simulation.
  const [focusedOnly, setFocusedOnly] = useState(false);

  const data = useMemo(
    () =>
      graph
        ? buildOntologyGraphData(graph, {
            showInheritance,
            showUndeclared,
            width: layoutWidth,
            height: layoutHeight,
          })
        : {
            nodes: [] as OntologyGraphNode[],
            links: [] as OntologyGraphLink[],
            maxEntities: 0,
            maxRelations: 0,
            hasEmptyClass: false,
          },
    [graph, showInheritance, showUndeclared, layoutWidth, layoutHeight],
  );

  // The drill-down and the panel that shows it live in the host column (see
  // OntologyDetailPanel): the canvas only reports which class was clicked.

  const activeId = selected ?? highlightClass ?? null;
  const activeNode = useMemo(
    () => data.nodes.find((node) => node.id === activeId) ?? null,
    [data, activeId],
  );

  // Hover emphasises the hovered class; a selection (or a controlled highlight)
  // pins the same emphasis so a click survives mouse-out. The 1-hop set is
  // resolved by class id, not by the simulation's per-node link list.
  const focusNode = hovered ?? activeNode;
  const emphasis = useMemo(() => {
    // An edge the reader is pointing at or has clicked IS the emphasis: it is
    // the thing being asked about, and it lights up both of its endpoints.
    const activeLink = hoveredLink ?? selectedLink;
    if (activeLink) {
      const sourceId = linkEndpointId(activeLink.source);
      const targetId = linkEndpointId(activeLink.target);
      return {
        nodes: new Set(
          data.nodes.filter(
            (node) => node.id === sourceId || node.id === targetId,
          ),
        ),
        links: new Set([activeLink]),
      };
    }
    if (!focusNode) return null;
    const nodeIds = new Set<string>([focusNode.id]);
    const links = new Set<OntologyGraphLink>();
    data.links.forEach((link) => {
      const sourceId = linkEndpointId(link.source);
      const targetId = linkEndpointId(link.target);
      if (sourceId !== focusNode.id && targetId !== focusNode.id) return;
      links.add(link);
      nodeIds.add(sourceId);
      nodeIds.add(targetId);
    });
    return {
      nodes: new Set(data.nodes.filter((node) => nodeIds.has(node.id))),
      links,
    };
  }, [data, focusNode, hoveredLink, selectedLink]);

  useEffect(() => {
    hasFittedRef.current = false;
  }, [data]);

  // The layout is computed, not simulated. The engine's default forces would
  // pull the rings apart on the one tick it runs, and a link force would drag a
  // node back the moment the reader let go of it — so they are removed, and the
  // layout's positions are simply where the nodes are. Dragging then behaves:
  // the library pins the node under the pointer and it stays where it is put.
  useEffect(() => {
    const fg = fgRef.current;
    if (!fg || !hasDimensions) return;
    fg.d3Force('link', null);
    fg.d3Force('charge', null);
    fg.d3Force('center', null);
  }, [hasDimensions, data]);

  // Dragging is the reader's tool for untangling a dense graph, so there has to
  // be a way back to the computed layout.
  const resetLayout = useCallback(() => {
    const positions = computeOntologyLayout(
      data.nodes,
      data.links,
      layoutWidth,
      layoutHeight,
    );
    data.nodes.forEach((node) => {
      const point = positions.get(node.id);
      if (!point) return;
      node.x = point.x;
      node.y = point.y;
      // Unpin: a node the reader dragged carries `fx`/`fy`, and leaving them
      // would make it ignore the layout it is being reset to.
      node.fx = undefined;
      node.fy = undefined;
    });
    fgRef.current?.zoomToFit(400, 60);
  }, [data, layoutHeight, layoutWidth]);

  useEffect(() => {
    setSelected(highlightClass ?? null);
  }, [highlightClass]);

  const handleEngineStop = useCallback(() => {
    if (!hasFittedRef.current && fgRef.current) {
      fgRef.current.zoomToFit(400, 60);
      hasFittedRef.current = true;
    }
  }, []);

  const handleNodeClick = useCallback(
    (node: OntologyNodeObject) => {
      setSelected(node.id);
      // A class click means the next edge click is a fresh question.
      setSelectedLink(null);
    },
    [],
  );

  const handleLinkHover = useCallback((link: OntologyGraphLink | null) => {
    setHoveredLink(link);
  }, []);

  const handleLinkClick = useCallback(
    (link: OntologyGraphLink) => {
      // Inheritance is a schema statement with no instances behind it, so there
      // is nothing to open.
      if (link.kind !== 'property') return;
      setSelectedLink(link);
      // The panel is class-scoped, and a bundle's properties are declared by its
      // SOURCE class (the domain) — so that is the class whose panel answers
      // "what is this edge?": it lists each of the bundle's properties with its
      // own count and its own drill-down.
      const sourceId = linkEndpointId(link.source);
      setSelected(sourceId);
    },
    [],
  );

  const handleNodeHover = useCallback((node: OntologyNodeObject | null) => {
    setHovered(node ?? null);
  }, []);

  const nodeColor = useCallback(
    (node: OntologyNodeObject) => {
      const typed = node as OntologyGraphNode;
      const base = typed.declared ? ClassColor : UndeclaredClassColor;
      if (!emphasis) return base;
      return emphasis.nodes.has(typed) ? base : withAlpha(base, DimmedAlpha);
    },
    [emphasis],
  );

  const nodeVal = useCallback(
    (node: OntologyNodeObject) => (node as OntologyGraphNode).radius,
    [],
  );

  const linkColor = useCallback(
    (link: OntologyGraphLink) => {
      // Inheritance is not a counted property edge, so it gets its own colour
      // and reads stronger than the property edges it sits among.
      const isInheritance = link.kind === 'inheritance';
      const base = isInheritance
        ? InheritanceColor
        : link.declared
          ? EdgeColor
          : UndeclaredClassColor;
      if (!emphasis) return withAlpha(base, isInheritance ? 0.85 : 0.55);
      return emphasis.links.has(link)
        ? withAlpha(HighlightColor, HighlightAlpha)
        : withAlpha(base, DimmedAlpha);
    },
    [emphasis],
  );

  const linkWidth = useCallback(
    (link: OntologyGraphLink) =>
      emphasis?.links.has(link) ? link.width + 1 : link.width,
    [emphasis],
  );

  const linkCurvature = useCallback(
    (link: OntologyGraphLink) => link.curvature,
    [],
  );

  const getNodeLabel = useCallback(
    (node: OntologyNodeObject) => {
      const typed = node as OntologyGraphNode;
      const description = typed.description ? `\n${typed.description}` : '';
      // The label is what a reader calls the class; the name is what the data
      // calls it. Both are shown when they differ, because a filter value must
      // be the name.
      const heading =
        typed.label === typed.id ? typed.id : `${typed.label} (${typed.id})`;
      return `${heading} · ${t('knowledgeCompilation.ontologyInstanceCount', {
        count: typed.entities,
        defaultValue: '{{count}} instance(s)',
      })}${description}`;
    },
    [t],
  );

  /** The full property list of a bundle, for the edge tooltip. */
  const getLinkLabel = useCallback(
    (link: OntologyGraphLink) => {
      if (link.kind !== 'property' || link.types.length === 0) return '';
      const counts = t('knowledgeCompilation.ontologyInstanceCount', {
        count: link.relations,
        defaultValue: '{{count}} instance(s)',
      });
      return `${link.types.join(', ')} · ${counts}`;
    },
    [t],
  );

  // Space claimed by a label during the frame being painted. Decluttering is the
  // only way to keep edge names readable on a near-complete graph: with the
  // classes on rings, every chord between outer classes crosses the middle, so
  // the curve midpoints — exactly where a label wants to sit — are the crowded
  // part of the canvas. Labels that find no room are dropped; the same names are
  // one hover away (linkLabel) and listed in the panel.
  const claimedRef = useRef<LabelBox[]>([]);

  /** Where each class name ended up this frame; null means "could not fit". */
  const nodeLabelsRef = useRef<Map<string, NodeLabelPlacement | null>>(
    new Map(),
  );

  /**
   * labelGeometry places a link's name: on its own curve (so parallel edges do
   * not share a label position), rotated to follow the edge (so a stack of them
   * does not read as one blob), and kept upright.
   */
  const labelGeometry = useCallback(
    (
      link: OntologyGraphLink,
      ctx: CanvasRenderingContext2D,
      globalScale: number,
    ): LinkLabel | null => {
      if (!link.label) return null;
      const source = link.source as OntologyNodeObject;
      const target = link.target as OntologyNodeObject;
      if (typeof source !== 'object' || typeof target !== 'object') return null;
      const sx = source.x ?? 0;
      const sy = source.y ?? 0;
      const tx = target.x ?? 0;
      const ty = target.y ?? 0;
      const dx = tx - sx;
      const dy = ty - sy;
      const distance = Math.hypot(dx, dy) || 1;
      const nx = -dy / distance;
      const ny = dx / distance;
      const bulge = link.curvature * distance;
      const highlighted = emphasis?.links.has(link) ?? false;
      const fontSize = Math.max(2.6, 9 / globalScale);
      // A bundle can carry five property names. Printing all of them on every
      // edge is what turns a dense ontology into unreadable text, so an
      // unfocused bundle shows its first name plus a count — the full list is
      // one hover away (linkLabel) and spelled out in the panel.
      const text =
        highlighted || link.types.length <= 1
          ? link.label
          : `${link.types[0]} +${link.types.length - 1}`;
      let angle = Math.atan2(dy, dx);
      if (angle > Math.PI / 2) angle -= Math.PI;
      if (angle < -Math.PI / 2) angle += Math.PI;

      ctx.save();
      ctx.font = `${highlighted ? '600 ' : ''}${fontSize}px Sans-Serif`;
      const width = ctx.measureText(text).width;
      ctx.restore();

      return {
        // A quadratic curve with its control point at mid + n*bulge passes
        // through mid + n*bulge/2 — that half is what the eye reads as "on the
        // line", and getting it wrong is what stacks labels off their edges.
        x: (sx + tx) / 2 + nx * (bulge / 2),
        y: (sy + ty) / 2 + ny * (bulge / 2),
        angle,
        text,
        fontSize,
        highlighted,
        width,
        height: fontSize * 1.35,
      };
    },
    [emphasis],
  );

  // Property names sit on the edges in the canvas: they are the ontology's
  // vocabulary, and without them the graph is just an unlabelled shape.
  const paintLink = useCallback(
    (
      link: OntologyGraphLink,
      ctx: CanvasRenderingContext2D,
      globalScale: number,
    ) => {
      // Hiding, not dimming: a dimmed edge still draws, and on a dense template
      // the mesh itself is the problem, not its contrast. Inheritance is exempt —
      // it is what the reader navigates by.
      if (focusedOnly && !emphasis?.links.has(link) && link.kind === 'property') {
        return;
      }
      const source = link.source as OntologyNodeObject;
      const target = link.target as OntologyNodeObject;
      if (typeof source !== 'object' || typeof target !== 'object') return;
      const sx = source.x ?? 0;
      const sy = source.y ?? 0;
      const tx = target.x ?? 0;
      const ty = target.y ?? 0;
      const dx = tx - sx;
      const dy = ty - sy;
      const distance = Math.hypot(dx, dy) || 1;
      const color = linkColor(link);
      const width = linkWidth(link) / globalScale;
      // rdfs:subClassOf is drawn dashed: it is a schema statement, and it has to
      // be distinguishable from a property even when both connect the same pair.
      const isInheritance = link.kind === 'inheritance';

      ctx.save();
      ctx.strokeStyle = color;
      ctx.fillStyle = color;
      ctx.lineWidth = width;
      if (isInheritance) {
        ctx.setLineDash([4, 3]);
      }

      const sourceRadius = (source as OntologyGraphNode).radius ?? 0;
      const targetRadius = (target as OntologyGraphNode).radius ?? 0;
      // Perpendicular offset puts each parallel edge on its own arc.
      const nx = -dy / distance;
      const ny = dx / distance;
      const bulge = link.curvature * distance;

      if (Math.abs(sx - tx) < 0.01 && Math.abs(sy - ty) < 0.01) {
        // Self-referencing property: a small loop above the node.
        ctx.beginPath();
        ctx.arc(sx, sy - sourceRadius - 8, 10, 0, Math.PI * 2);
        ctx.stroke();
      } else {
        const ux = dx / distance;
        const uy = dy / distance;
        const startX = sx + ux * sourceRadius;
        const startY = sy + uy * sourceRadius;
        const endX = tx - ux * (targetRadius + 6);
        const endY = ty - uy * (targetRadius + 6);
        ctx.beginPath();
        ctx.moveTo(startX, startY);
        ctx.quadraticCurveTo(
          (startX + endX) / 2 + nx * bulge,
          (startY + endY) / 2 + ny * bulge,
          endX,
          endY,
        );
        ctx.stroke();

        // Arrow head marks the property's direction (domain -> range).
        const angle = Math.atan2(endY - startY, endX - startX);
        const arrow = 5 / globalScale + 2;
        ctx.beginPath();
        ctx.moveTo(endX, endY);
        ctx.lineTo(
          endX - arrow * Math.cos(angle - Math.PI / 7),
          endY - arrow * Math.sin(angle - Math.PI / 7),
        );
        ctx.lineTo(
          endX - arrow * Math.cos(angle + Math.PI / 7),
          endY - arrow * Math.sin(angle + Math.PI / 7),
        );
        ctx.closePath();
        if (isInheritance) {
          // Hollow head: an inheritance edge carries no count, so it should not
          // read as a filled property arrow.
          ctx.stroke();
        } else {
          ctx.fill();
        }
      }

      const label = labelGeometry(link, ctx, globalScale);
      if (label) {
        // An emphasised edge always prints its name: the reader asked for it
        // (hover or click). Everything else yields to what is already on the
        // canvas, so the names that survive are the readable ones.
        const collides = claimedRef.current.some((box) =>
          labelsOverlap(box, label),
        );
        if (!collides || label.highlighted) {
          if (!collides) claimedRef.current.push(label);
          ctx.save();
          ctx.translate(label.x, label.y);
          ctx.rotate(label.angle);
          ctx.font = `${label.highlighted ? '600 ' : ''}${label.fontSize}px Sans-Serif`;
          ctx.textAlign = 'center';
          ctx.textBaseline = 'middle';
          // A halo, because a name that crosses its own line is exactly the
          // case this whole pass exists for.
          ctx.lineWidth = 3 / globalScale;
          ctx.strokeStyle = ctx.canvas.style.backgroundColor || 'rgba(0,0,0,0.55)';
          ctx.strokeText(label.text, 0, 0);
          ctx.fillText(label.text, 0, 0);
          ctx.restore();
        }
      }
      ctx.restore();
    },
    [emphasis, labelGeometry, linkColor, linkWidth, focusedOnly],
  );

  // Reset the claimed space once per frame and let the emphasised edges claim
  // theirs FIRST: the painter walks the links in array order, so without this a
  // low-count edge drawn early could take the spot the hovered one needs.
  /**
   * nodeLabelBox measures a class label as drawn from one anchor, so the
   * declutter pass can reason about it like any other box.
   */
  const nodeLabelBox = useCallback(
    (
      node: OntologyGraphNode,
      ctx: CanvasRenderingContext2D,
      side: { x: number; y: number; align: CanvasTextAlign; baseline: CanvasTextBaseline },
      text: string,
      count: string | null,
    ): NodeLabelPlacement => {
      const x = (node.x ?? 0) + side.x;
      const y = (node.y ?? 0) + side.y;
      ctx.save();
      ctx.font = '600 11px Sans-Serif';
      const textWidth = ctx.measureText(text).width;
      ctx.restore();
      const width = Math.max(textWidth, 14);
      // The name plus the count line printed under it.
      const height = count ? 24 : 13;
      const cx =
        side.align === 'center'
          ? x
          : side.align === 'left'
            ? x + width / 2
            : x - width / 2;
      const cy =
        side.baseline === 'top'
          ? y + height / 2
          : side.baseline === 'bottom'
            ? y - height / 2
            : y;
      return {
        x,
        y,
        align: side.align,
        baseline: side.baseline,
        text,
        count,
        box: {
          x: cx,
          y: cy,
          width,
          height,
          angle: 0,
          owner: `node:${node.id}`,
        },
      };
    },
    [],
  );

  const handleRenderFramePre = useCallback(
    (ctx: CanvasRenderingContext2D, globalScale: number) => {
      claimedRef.current = [];
      // 1. Every disc claims its own ring of space, so no NAME of any kind is
      //    ever printed on top of a class.
      data.nodes.forEach((node) => {
        const radius = node.radius + 2;
        claimedRef.current.push({
          x: node.x ?? 0,
          y: node.y ?? 0,
          width: radius * 2,
          height: radius * 2,
          angle: 0,
          owner: `disc:${node.id}`,
        });
      });
      // 2. The edge the reader is pointing at claims its name first: hover and
      //    click are deliberate, so they win over whatever claims space later.
      if (emphasis) {
        data.links.forEach((link) => {
          if (!emphasis.links.has(link)) return;
          const label = labelGeometry(link, ctx, globalScale);
          if (label) claimedRef.current.push(label);
        });
      }
      // 3. Class names pick the first side of their disc that is still free.
      //    Deciding it here — once, before anything is painted — is what keeps
      //    the placement stable for the frame; deciding it inside the painter
      //    would make it depend on how many edge labels happened to be drawn
      //    first.
      const placements = new Map<string, NodeLabelPlacement | null>();
      data.nodes.forEach((node) => {
        const emphasized =
          !emphasis || emphasis.nodes.has(node) || hovered === node;
        if (!emphasized) {
          // A dimmed class keeps its name off the canvas: one focus at a time is
          // the whole point of focusing. With no focus every node is emphasised,
          // and then it is the declutter pass that decides.
          placements.set(node.id, null);
          return;
        }
        const count = node.entities > 0 ? String(node.entities) : null;
        let placed: NodeLabelPlacement | null = null;
        for (const side of NodeLabelSides(node.radius)) {
          const candidate = nodeLabelBox(node, ctx, side, node.label, count);
          const busy = claimedRef.current.some(
            (box) =>
              box.owner !== candidate.box.owner &&
              labelsOverlap(box, candidate.box),
          );
          // The node under the pointer always takes its first side: the reader
          // is asking about it, and it is how they tell two overlapping classes
          // apart.
          if (busy && hovered !== node) continue;
          placed = candidate;
          break;
        }
        if (placed) claimedRef.current.push(placed.box);
        placements.set(node.id, placed);
      });
      nodeLabelsRef.current = placements;
    },
    [data, emphasis, hovered, labelGeometry, nodeLabelBox],
  );

  // Class names are painted in one pass at the END, after the edges: they belong
  // on top of the lines they sit between. Their placement was decided — and their
  // space reserved — before a single edge label was drawn, so nothing can land on
  // them afterwards.
  const handleRenderFramePost = useCallback(
    (ctx: CanvasRenderingContext2D, _globalScale: number) => {
      data.nodes.forEach((node) => {
        const placement = nodeLabelsRef.current.get(node.id);
        if (!placement) return;
        const color = nodeColor(node);
        const countOffset =
          placement.baseline === 'top'
            ? 12
            : placement.baseline === 'bottom'
              ? -12
              : 10;
        ctx.save();
        ctx.textAlign = placement.align;
        ctx.textBaseline = placement.baseline;
        ctx.fillStyle = color;
        ctx.font = `${node.entities > 0 ? '600 ' : ''}11px Sans-Serif`;
        // The printed text is the label; the name is what filters and the
        // drill-down are keyed by, and it is shown in the panel instead.
        ctx.fillText(placement.text, placement.x, placement.y);
        if (placement.count) {
          ctx.font = '9px Sans-Serif';
          ctx.fillStyle = withAlpha(color, 0.75);
          ctx.fillText(
            placement.count,
            placement.x,
            placement.y + countOffset,
          );
        }
        ctx.restore();
      });
    },
    [data, nodeColor],
  );

  const paintNode = useCallback(
    (node: OntologyNodeObject, ctx: CanvasRenderingContext2D) => {
      const typed = node as OntologyGraphNode;
      const x = node.x ?? 0;
      const y = node.y ?? 0;
      const radius = typed.radius;
      const color = nodeColor(node);

      ctx.save();
      ctx.beginPath();
      ctx.arc(x, y, radius, 0, Math.PI * 2);
      ctx.fillStyle = color;
      ctx.fill();
      if (!typed.declared) {
        // Undeclared classes are outlined to read as vocabulary drift.
        ctx.setLineDash([2, 2]);
        ctx.lineWidth = 1;
        ctx.strokeStyle = color;
        ctx.stroke();
        ctx.setLineDash([]);
      }
      ctx.restore();
    },
    [emphasis, hovered, nodeColor],
  );

  const nodeCanvasObjectMode = useCallback(() => 'replace' as const, []);

  const matches = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) return [];
    // Both the printed label and the name are searchable: a reader looks for
    // what the canvas shows, a filter is keyed by the name.
    return data.nodes.filter(
      (node) =>
        node.label.toLowerCase().includes(keyword) ||
        node.id.toLowerCase().includes(keyword),
    );
  }, [data, query]);

  const focusOn = useCallback(
    (classType: string) => {
      setSelected(classType);
      const node = data.nodes.find((item) => item.id === classType);
      const target = node as OntologyNodeObject | undefined;
      if (node && fgRef.current && target?.x !== undefined) {
        fgRef.current.centerAt(target.x, target.y, 400);
        fgRef.current.zoom(3, 400);
      }
    },
    [data],
  );

  const selectedNode = useMemo(
    () => data.nodes.find((node) => node.id === selected) ?? null,
    [data, selected],
  );

  const selectedParents = useMemo(() => {
    if (!selected) return [];
    return data.links
      .filter(
        (link) =>
          link.kind === 'inheritance' &&
          linkEndpointId(link.target) === selected,
      )
      .map((link) => linkEndpointId(link.source));
  }, [data, selected]);

  const selectedChildren = useMemo(() => {
    if (!selected) return [];
    return data.links
      .filter(
        (link) =>
          link.kind === 'inheritance' &&
          linkEndpointId(link.source) === selected,
      )
      .map((link) => linkEndpointId(link.target));
  }, [data, selected]);

  /** classLabel is what the canvas prints; the name stays the identifier. */
  const classLabel = useCallback(
    (id: string) => data.nodes.find((node) => node.id === id)?.label ?? id,
    [data],
  );

  // The host owns the panel state; the effect below only reports the selection.
  // Its cleanup MUST NOT publish null, because React runs a cleanup on every
  // dependency change — and this canvas re-renders on resize, on a toggle and
  // whenever the graph data is rebuilt. A null published in between unmounts the
  // panel and it remounts a frame later, which reads as "the panel disappears
  // when I click something". Only a real unmount clears it, via the ref so the
  // empty-dep effect still calls the latest callback.
  const detailChangeRef = useRef(onClassDetailChange);
  detailChangeRef.current = onClassDetailChange;
  useEffect(() => () => detailChangeRef.current?.(null), []);

  // Publish the selected class so the host renders the detail column. The panel
  // is not drawn here: the canvas lives in the artifact column, and a panel drawn
  // inside it is squashed (or covered) as soon as the column divider moves.
  useEffect(() => {
    if (!onClassDetailChange) return;
    if (!selectedNode) {
      onClassDetailChange(null);
      return;
    }
    const ref = (id: string) => ({ name: id, label: classLabel(id) });
    // Read from the DECLARATION, not from the canvas links: the canvas bundles
    // several properties into one edge, while the panel must list each property
    // with its own count — that is the number a reader acts on.
    const properties: OntologyDetailProperty[] = (graph?.properties ?? [])
      .filter(
        (item) =>
          item.source === selectedNode.id || item.target === selectedNode.id,
      )
      .map((item) => ({
        type: item.type,
        label: item.label || item.type,
        source: item.source,
        target: item.target,
        relations: item.relations ?? 0,
        declared: item.declared !== false,
      }));
    onClassDetailChange({
      className: selectedNode.id,
      label: selectedNode.label,
      description: selectedNode.description,
      entities: selectedNode.entities,
      declared: selectedNode.declared,
      parents: selectedParents.map(ref),
      children: selectedChildren.map(ref),
      // `inherited_from` is a display-only field, so it is resolved to the label
      // the canvas prints; the identifier is the class name shown beside it.
      attributes: selectedNode.attributes.map((attribute) => ({
        ...attribute,
        inherited_from: attribute.inherited_from
          ? classLabel(attribute.inherited_from)
          : undefined,
      })),
      properties,
      // The edge that was clicked, so the panel can point at the rows that edge
      // stands for: the panel is class-scoped, and without this a click on a
      // bundle opens a list of the class's properties with nothing to say which
      // of them the reader just asked about.
      focusedProperties: selectedLink?.types,
      datasetId,
      documentId,
      templateId,
      onSelect: focusOn,
      onClose: () => setSelected(null),
    });
  }, [
    onClassDetailChange,
    selectedLink,
    selectedNode,
    graph,
    selectedParents,
    selectedChildren,
    classLabel,
    focusOn,
    datasetId,
    documentId,
    templateId,
  ]);


  return (
    // The root stays a flex ROW even with a single child: the canvas container
    // below carries `flex-1`, which only resolves to a height inside a flex
    // parent. Without it the container measures 0 and nothing is painted at all
    // — the overlays still draw, so the page looks blank except for the stats.
    <div className={cn('flex h-full min-h-0', className)}>
      <div
        ref={containerRef}
        // `min-w-0` and `overflow-hidden` are load-bearing, not cosmetic.
        //
        // A flex item's automatic minimum size (`min-width: auto`) applies only
        // while its overflow is visible — and this item's content is a canvas
        // whose intrinsic width IS the width measured last frame. So when the
        // host opens the detail column next door, this column could not shrink:
        // it stayed at its old width, overflowed the row and painted OVER that
        // column. There it also swallowed the clicks — a click on a panel button
        // landed on the canvas background, which clears the selection, so the
        // button "did nothing" and the panel vanished, with no request and no
        // error. `overflow-hidden` removes the automatic minimum (letting the
        // column shrink) and clips whatever is left; `min-w-0` states the same
        // requirement so removing the clip cannot bring the bug back.
        className="relative flex-1 min-w-0 min-h-0 h-full overflow-hidden"
      >
        {/* The canvas sits in a column that the detail column can squeeze, so
            every overlay is capped to the container: an overlay wider than its
            column spills into the neighbouring panel and reads as "the panel
            covers the stats". */}
        <div className="absolute left-2 top-2 z-10 flex max-w-[calc(100%-1rem)] flex-col gap-2">
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('knowledgeCompilation.ontologySearchClass', {
              defaultValue: 'Find a class…',
            })}
            className="w-44 rounded-md border border-border-button bg-bg-card px-2 py-1 text-xs outline-none"
          />
          {matches.length > 0 && (
            <div className="max-h-40 w-44 overflow-auto rounded-md border border-border-button bg-bg-card p-1 text-xs shadow">
              {matches.map((node) => (
                <button
                  key={node.id}
                  type="button"
                  className="block w-full truncate rounded px-1 py-0.5 text-left hover:bg-bg-base"
                  onClick={() => focusOn(node.id)}
                >
                  {node.label}
                  <span className="ml-1 text-text-secondary">
                    {node.entities}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>


        {hasDimensions && (
          <ForceGraph2D
            ref={fgRef}
            width={dimensions.width}
            height={dimensions.height}
            graphData={data}
            nodeRelSize={1}
            nodeId="id"
            nodeVal={nodeVal}
            nodeColor={nodeColor}
            nodeLabel={getNodeLabel}
            nodeCanvasObject={paintNode}
            nodeCanvasObjectMode={nodeCanvasObjectMode}
            linkColor={linkColor}
            linkWidth={linkWidth}
            linkCurvature={linkCurvature}
            linkCanvasObject={paintLink}
            linkCanvasObjectMode={nodeCanvasObjectMode}
            linkLabel={getLinkLabel}
            onRenderFramePre={handleRenderFramePre}
            onRenderFramePost={handleRenderFramePost}
            onLinkClick={handleLinkClick}
            onLinkHover={handleLinkHover}
            onNodeClick={handleNodeClick}
            onNodeHover={handleNodeHover}
            onBackgroundClick={() => setSelected(null)}
            onEngineStop={handleEngineStop}
            // Every node is pinned by the ring layout, so the simulation has
            // nothing left to solve. Freezing it the way the reference viewer
            // freezes a saved layout keeps the picture from jiggling: the
            // distance between two classes is a statement about the ontology, not
            // an artefact of where the physics stopped.
            cooldownTicks={30}
            autoPauseRedraw={false}
            d3AlphaDecay={1}
            d3VelocityDecay={1}
          />
        )}

        {/* ONE bottom bar, not two absolutely placed blocks: both were pinned
            with `bottom-2` at the same stacking level, so once the detail column
            narrows the canvas the pitfall list (right) grew leftwards and painted
            OVER the legend (left) — the toggles and the list became one
            unreadable pile. One row cannot overlap itself, and each half keeps
            its own surface so the canvas does not show through it either. */}
        <div className="absolute inset-x-2 bottom-2 z-10 flex items-end justify-between gap-2">
          <div className="flex min-w-0 flex-wrap items-center gap-3 rounded-md border border-border-button bg-bg-card px-2 py-1 text-xs text-text-secondary">
            <span className="flex items-center gap-1">
              <span
                className="inline-block h-2 w-2 rounded-full"
                style={{ background: ClassColor }}
              />
              {t('knowledgeCompilation.ontologyDeclaredClass', {
                defaultValue: 'Declared class',
              })}
            </span>
            <span className="flex items-center gap-1">
              <span
                className="inline-block h-2 w-2 rounded-full"
                style={{ background: UndeclaredClassColor }}
              />
              {t('knowledgeCompilation.ontologyUndeclaredClass', {
                defaultValue: 'Observed, not declared',
              })}
            </span>
            <label className="flex cursor-pointer items-center gap-1">
              <input
                type="checkbox"
                checked={showInheritance}
                onChange={(event) => setShowInheritance(event.target.checked)}
              />
              <span
                className="inline-block h-0 w-4 border-t-2 border-dashed"
                style={{ borderColor: InheritanceColor }}
              />
              {t('knowledgeCompilation.ontologyShowInheritance', {
                defaultValue: 'Inheritance',
              })}
            </label>
            <label className="flex cursor-pointer items-center gap-1">
              <input
                type="checkbox"
                checked={showUndeclared}
                onChange={(event) => setShowUndeclared(event.target.checked)}
              />
              {t('knowledgeCompilation.ontologyShowUndeclared', {
                defaultValue: 'Show undeclared classes and properties',
              })}
            </label>
            <label className="flex cursor-pointer items-center gap-1">
              <input
                type="checkbox"
                checked={focusedOnly}
                onChange={(event) => setFocusedOnly(event.target.checked)}
              />
              {t('knowledgeCompilation.ontologyFocusedOnly', {
                defaultValue: 'Only the focused class’s edges',
              })}
            </label>
            {/* Classes can be dragged out of the crowd, so there must be a way
                back to the layout the ontology itself implies. */}
            <button
              type="button"
              className="rounded border border-border-button px-1.5 py-0.5 hover:bg-bg-base"
              onClick={resetLayout}
            >
              {t('knowledgeCompilation.ontologyResetLayout', {
                defaultValue: 'Reset layout',
              })}
            </button>
          </div>

          {(graph?.pitfalls?.length ?? 0) > 0 && (
            // Grouped by category and capped, the way the reference detector
            // reports: fifteen flat sentences on a canvas overlay is a wall, while
            // "Logical · 2" tells the reader what kind of problem they are looking
            // at before they read a word of it.
            <div className="flex max-h-48 min-w-0 flex-col gap-1 overflow-auto rounded-md border border-border-button bg-bg-card px-2 py-1 text-xs">
              {PitfallCategoryOrder.map((category) => {
                const found = (graph?.pitfalls ?? []).filter(
                  (pitfall) => (pitfall.category ?? 'structural') === category,
                );
                if (found.length === 0) return null;
                return (
                  <div key={category} className="flex flex-col gap-1">
                    <span className="text-text-secondary">
                      {pitfallCategoryLabel(category, t)} · {found.length}
                    </span>
                    {found.map((pitfall) => (
                      <div
                        key={pitfall.code}
                        // The service's own sentence stays in the tooltip: it names
                        // the classes and properties, so the panel stays readable
                        // and nothing is lost.
                        title={pitfall.message}
                        className={cn(
                          'rounded-md border bg-bg-card px-2 py-1',
                          pitfall.severity === 'error'
                            ? 'border-red-400 text-red-500'
                            : 'border-border-button text-text-secondary',
                        )}
                      >
                        {pitfallLabel(pitfall.code, t)}
                        {pitfall.subjects?.length > 0 && (
                          <span className="ml-1 opacity-70">
                            ({pitfall.subjects.slice(0, 3).join(', ')}
                            {pitfall.subjects.length > 3
                              ? ` +${pitfall.subjects.length - 3}`
                              : ''}
                            )
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

    </div>
  );
}

export default OntologyModelGraph;
