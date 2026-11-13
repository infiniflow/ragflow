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

import { ArtifactForceGraph } from '@/components/artifact-force-graph';
import { OntologyEvidenceSheet } from '@/components/compilation-template-group/ontology/evidence-sheet';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import type { IArtifactGraphEntity } from '@/interfaces/database/dataset';
import type {
  IOntologyGraphPage,
  IStructureGraphEntity,
  IStructureGraphRelation,
} from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  classColor,
  classGraphFrom,
  egoSubgraph,
  entitySlug,
  instanceGraphFrom,
  propertyOptionsFrom,
} from './adapters';
import { AboxDetail, type AboxNeighbour } from './detail';
import { useOntologyGraph, useOntologyRows } from './hooks';
import { AboxSearchBox } from './search-box';
import { EntityTable, RelationTable } from './tables';
import type { AboxClassOption, AboxPropertyOption, AboxTab } from './types';

export type {
  AboxClassOption,
  AboxPropertyOption,
  AboxRowKind,
  AboxTab,
} from './types';

interface OntologyAboxPanelProps {
  /** The dataset whose compiled rows are shown. Required: rows live in one. */
  datasetId: string;
  /** Set on a document page: narrows to the rows read from that document. */
  documentId?: string;
  /** Narrows to one compilation model's rows (and its declared vocabulary). */
  templateId?: string;
  /**
   * The classes the model declares. They are the filter's closed set, so a
   * reader picks from the vocabulary the model actually promises instead of
   * from whatever happened to land on the first page.
   */
  classes?: AboxClassOption[];
  properties?: AboxPropertyOption[];
  /** Opening selection — the class the reader clicked on the way in. */
  initialClass?: string;
  initialProperty?: string;
  initialSource?: string;
  initialTarget?: string;
  className?: string;
}

const RowTabs: AboxTab[] = ['classes', 'instances', 'entities', 'relations'];

/**
 * Which half of the ABox a row belongs to. `from` is required on an assertion
 * and absent on an instance, which is what makes it the discriminant (`name` is
 * optional on an instance, so `'name' in row` cannot narrow the union).
 */
const isRelationRow = (
  row: IStructureGraphEntity | IStructureGraphRelation,
): row is IStructureGraphRelation => 'from' in row;

/**
 * The ABox of one scope, read three ways.
 *
 * The three views are not variations of one another, they answer different
 * questions and have different cost models:
 *
 * - classes: what did the model collect, and how much of it. One bubble per
 *   class, drawn from the declared vocabulary — no query, so it opens instantly
 *   on a dataset with a hundred thousand rows.
 * - instances: what is inside one class, and what it connects to. A bounded
 *   subgraph (budget + hop walk), never the whole class: a canvas cannot page.
 * - relations: the assertions as rows, which is the only view that can reach
 *   the last one.
 *
 * The budget line under the canvas is part of the design, not a debug aid:
 * without it a 60-node sample reads as the entire ABox.
 */
export function OntologyAboxPanel({
  datasetId,
  documentId,
  templateId,
  classes = [],
  properties = [],
  initialClass,
  initialProperty,
  initialSource,
  initialTarget,
  className,
}: OntologyAboxPanelProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<AboxTab>(
    initialProperty ? 'relations' : initialClass ? 'instances' : 'classes',
  );
  const [entityClass, setEntityClass] = useState(initialClass ?? '');
  const [property, setProperty] = useState(initialProperty ?? '');
  const [hop, setHop] = useState(1);
  const [offset, setOffset] = useState(0);
  const [activeKey, setActiveKey] = useState('');
  const [search, setSearch] = useState('');
  const [highlightSlug, setHighlightSlug] = useState<string>();
  const [egoOnly, setEgoOnly] = useState(false);
  const [selected, setSelected] = useState<
    IStructureGraphEntity | IStructureGraphRelation | undefined
  >();
  const [evidence, setEvidence] = useState<{
    open: boolean;
    heading?: string;
    documentIds?: string[];
    chunkIds?: string[];
    quotes?: {
      quote: string;
      chunk_id: string;
      start?: number;
      end?: number;
    }[];
  }>({ open: false });

  // A filter change is a new list, not the next page of the old one: keeping
  // the offset would open the new filter on an empty page past its end.
  useEffect(() => {
    setOffset(0);
    setActiveKey('');
  }, [tab, entityClass, property, documentId, datasetId, templateId]);

  const graphQuery = useOntologyGraph({
    datasetId,
    documentId,
    templateId,
    class: entityClass,
    hop,
    enabled: tab === 'instances',
  });
  const graphPage = graphQuery.data?.data?.data as
    | IOntologyGraphPage
    | undefined;

  const rowsQuery = useOntologyRows({
    datasetId,
    documentId,
    templateId,
    kind: tab === 'relations' ? 'relation' : 'entity',
    class: entityClass,
    property,
    sourceType: initialSource,
    targetType: initialTarget,
    offset,
    enabled: tab === 'entities' || tab === 'relations',
  });
  const rowsPage = rowsQuery.data?.data?.data as
    | {
        entities?: IStructureGraphEntity[];
        relations?: IStructureGraphRelation[];
        total?: number;
      }
    | undefined;

  const classGraph = useMemo(
    () => classGraphFrom(classes, properties),
    [classes, properties],
  );
  const instanceGraph = useMemo(
    () => instanceGraphFrom(graphPage),
    [graphPage],
  );

  // The node the reader picked, and the 1-hop view around it. Neither costs a
  // request: the canvas already holds the induced subgraph, so an ego view is a
  // subset of what is on screen rather than a second read.
  const selectedSlug = useMemo(() => {
    if (!selected || isRelationRow(selected)) {
      return undefined;
    }
    return entitySlug(selected.type, selected.name ?? '');
  }, [selected]);

  const activeGraph = useMemo(() => {
    if (tab === 'classes') {
      return classGraph;
    }
    if (!instanceGraph) {
      return undefined;
    }
    return egoOnly ? egoSubgraph(instanceGraph, selectedSlug) : instanceGraph;
  }, [classGraph, egoOnly, instanceGraph, selectedSlug, tab]);

  const searchResults = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) {
      return [];
    }
    return (activeGraph?.entities ?? [])
      .filter(
        (entity) =>
          entity.name.toLowerCase().includes(term) ||
          (entity.type ?? '').toLowerCase().includes(term) ||
          entity.aliases.some((alias) => alias.toLowerCase().includes(term)),
      )
      .slice(0, 8);
  }, [activeGraph, search]);

  const pickFromSearch = (entity: IArtifactGraphEntity) => {
    setSearch('');
    if (tab === 'classes') {
      // A class result is a shortcut to its instances, same as clicking it.
      setEntityClass(entity.slug);
      setTab('instances');
      return;
    }
    setSelected(graphPage?.entities.find((item) => item.name === entity.name));
    setHighlightSlug(entity.slug);
  };

  // Declared vocabulary first (the closed set the model promises), then any
  // class the canvas met but the model never declared — vocabulary drift is
  // something the reader has to be able to select and look at.
  const classOptions = useMemo(() => {
    const seen = new Set(classes.map((item) => item.type));
    const options = classes.map((item) => ({
      label: item.label || item.type,
      value: item.type,
    }));
    (graphPage?.classes ?? []).forEach((type) => {
      if (type && !seen.has(type)) {
        seen.add(type);
        options.push({ label: `${type} (+?)`, value: type });
      }
    });
    return options;
  }, [classes, graphPage?.classes]);

  const propertyOptions = useMemo(
    () => propertyOptionsFrom(properties),
    [properties],
  );

  const neighbours: AboxNeighbour[] = useMemo(() => {
    const name = (selected as IStructureGraphEntity | undefined)?.name;
    if (!name || !graphPage) {
      return [];
    }
    const out: AboxNeighbour[] = [];
    graphPage.relations.forEach((relation) => {
      if (relation.from === name) {
        out.push({ type: relation.type ?? '', name: relation.to });
      } else if (relation.to === name) {
        out.push({ type: relation.type ?? '', name: relation.from });
      }
    });
    return out;
  }, [graphPage, selected]);

  const openEvidence = (
    row: IStructureGraphEntity | IStructureGraphRelation,
  ) => {
    setEvidence({
      open: true,
      heading: isRelationRow(row) ? `${row.from} → ${row.to}` : row.name,
      documentIds: row.document_ids,
      chunkIds: row.source_chunk_ids,
      quotes: row.evidence,
    });
  };

  const handleGraphNodeClick = (node: IArtifactGraphEntity) => {
    if (tab === 'classes') {
      // A class bubble is a door, not a leaf: there is nothing to read on it.
      setEntityClass(node.slug);
      setTab('instances');
      return;
    }
    const entity = graphPage?.entities.find((item) => item.name === node.name);
    setSelected(entity);
    // Picking a node is also "show me its surroundings": the graph dims what is
    // outside the 1-hop neighbourhood, which is the read a bubble explains.
    setHighlightSlug(node.slug);
  };

  const budget = graphPage
    ? t('knowledgeCompilation.aboxBudget', {
        defaultValue: 'Canvas {{drawn}} / {{total}} instances',
        drawn: graphPage.drawn,
        total: graphPage.total,
      })
    : '';

  return (
    // h-full is load-bearing: every caller hands the panel a definite height
    // (a flex item or a `h-full` section), but a block child does not inherit
    // it. Without it the section keeps only its header height, the `flex-1`
    // row below resolves to 0, and the force canvas — which sizes itself from
    // its container — draws in a sliver instead of the page.
    <section className={cn('flex h-full min-h-0 w-full flex-col', className)}>
      <header className="flex flex-none items-center gap-2 border-b border-border-button pb-2">
        <div className="flex overflow-hidden rounded-md border border-border-button">
          {RowTabs.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setTab(item)}
              className={cn(
                'h-7 px-3 text-xs',
                tab === item
                  ? 'bg-bg-base font-semibold'
                  : 'text-text-secondary hover:bg-bg-base',
              )}
            >
              {item === 'classes' &&
                t('knowledgeCompilation.aboxClassesTab', {
                  defaultValue: 'Classes',
                })}
              {item === 'instances' &&
                t('knowledgeCompilation.aboxInstancesTab', {
                  defaultValue: 'Instances',
                })}
              {item === 'entities' &&
                t('knowledgeCompilation.aboxRowsTab', {
                  defaultValue: 'Rows',
                })}
              {item === 'relations' &&
                t('knowledgeCompilation.aboxRelationsTab', {
                  defaultValue: 'Relations',
                })}
            </button>
          ))}
        </div>

        {(tab === 'instances' || tab === 'entities') &&
          classOptions.length > 0 && (
            <SelectWithSearch
              value={entityClass}
              onChange={(value) => setEntityClass(value ?? '')}
              options={classOptions}
              allowClear
              placeholder={t('knowledgeCompilation.aboxAllClasses', {
                defaultValue: 'All classes',
              })}
              triggerClassName="w-48"
            />
          )}

        {tab === 'relations' && propertyOptions.length > 0 && (
          <SelectWithSearch
            value={property}
            onChange={(value) => setProperty(value ?? '')}
            options={propertyOptions}
            allowClear
            placeholder={t('knowledgeCompilation.aboxAllProperties', {
              defaultValue: 'All properties',
            })}
            triggerClassName="w-48"
          />
        )}

        <div className="ml-auto flex items-center gap-3 text-xs text-text-secondary">
          {tab === 'instances' && graphPage && (
            <span className="tabular-nums">
              {budget}
              {graphPage.truncated && (
                <button
                  type="button"
                  className="ml-2 text-text-accent hover:underline"
                  onClick={() => setTab('entities')}
                >
                  {t('knowledgeCompilation.aboxOpenRemainder', {
                    defaultValue: 'Rest in list →',
                  })}
                </button>
              )}
            </span>
          )}
          {tab === 'instances' && (
            <>
              {/* Two different expansions, two different buttons. This one
                  widens the walk to one more ring of classes (a new request);
                  the ego toggle below narrows the canvas to the selected
                  node's drawn neighbourhood (no request). */}
              <button
                type="button"
                onClick={() => setHop(hop >= 2 ? 1 : 2)}
                className={cn(
                  'h-7 rounded border border-border-button px-2',
                  hop >= 2 && 'bg-bg-base font-semibold',
                )}
                title={t('knowledgeCompilation.aboxExpandHopHint', {
                  defaultValue:
                    'Widens the walk by one ring of neighbouring classes',
                })}
              >
                {t('knowledgeCompilation.aboxExpandHop', {
                  defaultValue: 'One more ring',
                })}
              </button>
              <button
                type="button"
                disabled={!selectedSlug}
                onClick={() => setEgoOnly(!egoOnly)}
                className={cn(
                  'h-7 rounded border border-border-button px-2 disabled:opacity-40',
                  egoOnly && 'bg-bg-base font-semibold',
                )}
                title={t('knowledgeCompilation.aboxEgoOnlyHint', {
                  defaultValue:
                    'Keeps only the selected node and the neighbours this canvas already drew',
                })}
              >
                {t('knowledgeCompilation.aboxEgoOnly', {
                  defaultValue: 'One hop only',
                })}
              </button>
            </>
          )}
        </div>
      </header>

      {/* The canvas tabs share one row (graph + detail panel); the row tabs stack
          their table above the pager. One container, two directions — a single
          `flex` row put the pager beside the table as a narrow column. */}
      <div
        className={cn(
          'flex min-h-0 flex-1',
          (tab === 'entities' || tab === 'relations') && 'flex-col',
        )}
      >
        {tab === 'classes' && (
          <div className="relative min-h-0 flex-1">
            <ArtifactForceGraph
              show
              data={classGraph}
              getNodeId={(node) => node.slug}
              getNodeColor={(node) => classColor(node.type)}
              getNodeLabel={(node) => node.name}
              getNodeRadius={(node, minWeight, maxWeight) => {
                if (maxWeight <= minWeight) {
                  return 10;
                }
                const ratio =
                  (node.weight - minWeight) / (maxWeight - minWeight);
                return 7 + ratio * 18;
              }}
              onNodeClick={handleGraphNodeClick}
            />
            <AboxSearchBox
              value={search}
              onChange={setSearch}
              results={searchResults}
              onPick={pickFromSearch}
            />
          </div>
        )}

        {tab === 'instances' && (
          <>
            <div className="relative min-h-0 flex-1">
              {activeGraph && activeGraph.entities.length > 0 ? (
                <ArtifactForceGraph
                  show
                  data={activeGraph}
                  getNodeId={(node) => node.slug}
                  getNodeColor={(node) => classColor(node.type)}
                  getNodeLabel={(node) => node.name}
                  getNodeRadius={(node, minWeight, maxWeight) => {
                    if (maxWeight <= minWeight) {
                      return 6;
                    }
                    const ratio =
                      (node.weight - minWeight) / (maxWeight - minWeight);
                    return 5 + ratio * 11;
                  }}
                  onNodeClick={handleGraphNodeClick}
                  highlightNodeId={highlightSlug}
                  totalEntities={graphPage?.total}
                  returnedEntities={graphPage?.drawn}
                />
              ) : (
                <div className="flex h-full items-center justify-center text-xs text-text-secondary">
                  {t('knowledgeCompilation.ontologyInstancesEmpty', {
                    defaultValue: 'Nothing compiled for this filter yet.',
                  })}
                </div>
              )}
              <AboxSearchBox
                value={search}
                onChange={setSearch}
                results={searchResults}
                onPick={pickFromSearch}
              />
              {egoOnly && (
                <div className="pointer-events-none absolute left-3 top-12 rounded bg-bg-base/90 px-2 py-1 text-[10px] text-text-secondary">
                  {t('knowledgeCompilation.aboxEgoHint', {
                    defaultValue:
                      'Showing one hop around the selected node, from what this canvas drew.',
                  })}
                </div>
              )}
            </div>
            <AboxDetail
              row={selected}
              neighbours={neighbours}
              onOpenEvidence={openEvidence}
              onOpenInList={(row) => {
                // The list is the only view that can reach every row, so a
                // node the canvas could not draw leads here with its class
                // already filtered.
                setTab('entities');
                if ('from' in row) {
                  setEntityClass(row.source_type ?? entityClass);
                  return;
                }
                setEntityClass(row.type ?? entityClass);
              }}
            />
          </>
        )}

        {tab === 'entities' && (
          <EntityTable
            rows={rowsPage?.entities ?? []}
            total={rowsPage?.total ?? 0}
            offset={offset}
            onOffset={setOffset}
            activeKey={activeKey}
            onSelect={(row, key) => {
              setSelected(row);
              setActiveKey(key);
            }}
            onOpenEvidence={openEvidence}
          />
        )}

        {tab === 'relations' && (
          <RelationTable
            rows={rowsPage?.relations ?? []}
            total={rowsPage?.total ?? 0}
            offset={offset}
            onOffset={setOffset}
            activeKey={activeKey}
            onSelect={(row, key) => {
              setSelected(row);
              setActiveKey(key);
            }}
            onOpenEvidence={openEvidence}
          />
        )}
      </div>

      <OntologyEvidenceSheet
        open={evidence.open}
        onOpenChange={(open) => setEvidence((prev) => ({ ...prev, open }))}
        datasetId={datasetId}
        heading={evidence.heading}
        documentIds={evidence.documentIds}
        chunkIds={evidence.chunkIds}
        evidence={evidence.quotes}
      />
    </section>
  );
}

export default OntologyAboxPanel;
