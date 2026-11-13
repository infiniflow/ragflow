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

import { OntologyEvidenceView } from '@/components/compilation-template-group/ontology/evidence-sheet';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import type {
  IStructureGraphEntity,
  IStructureGraphRelation,
} from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import documentStructureService from '@/services/document-structure-service';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

/** One page of rows. A panel shows a page, not a dump. */
const PageSize = 30;

export type AboxRowKind = 'entity' | 'relation';

/** A declared class, as the filter offers it. */
export interface AboxClassOption {
  type: string;
  label?: string;
}

/** A declared object property; a polymorphic one appears once per edge. */
export interface AboxPropertyOption {
  type: string;
  label?: string;
  source: string;
  target: string;
}

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

/**
 * Which half of the ABox a row belongs to. `from` is required on an assertion
 * and absent on an instance, which is what makes it the discriminant (`name` is
 * optional on an instance, so `'name' in row` cannot narrow the union).
 */
const isRelationRow = (
  row: IStructureGraphEntity | IStructureGraphRelation,
): row is IStructureGraphRelation => 'from' in row;

/** The row identity used as a React key and to recognise the selected row. */
const rowKey = (
  row: IStructureGraphEntity | IStructureGraphRelation,
  index: number,
) =>
  isRelationRow(row)
    ? `r:${row.id ?? `${row.from}\u0000${row.type ?? ''}\u0000${row.to}`}`
    : `e:${row.id ?? row.name ?? index}`;

/** attributeEntries drops the empty values: neither should take a line. */
const attributeEntries = (entity: IStructureGraphEntity): [string, unknown][] =>
  Object.entries(entity.attributes ?? {}).filter(
    ([, value]) => value !== null && value !== undefined && value !== '',
  );

const formatAttributeValue = (value: unknown): string => {
  if (Array.isArray(value)) {
    return value.map((item) => formatAttributeValue(item)).join(', ');
  }
  if (value !== null && typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
};

/** The review state a row carries, when the review queue has touched it. */
function ReviewBadge({ status }: { status?: string }) {
  const { t } = useTranslation();
  if (status === 'accepted') {
    return (
      <Badge variant="success">
        {t('knowledgeCompilation.ontologyInspect.reviewAccepted')}
      </Badge>
    );
  }
  if (status === 'rejected') {
    return (
      <Badge variant="destructive">
        {t('knowledgeCompilation.ontologyInspect.reviewRejected')}
      </Badge>
    );
  }
  return null;
}

function DetailRow({ label, value }: { label: string; value: string }) {
  if (!value) return null;
  return (
    <div className="flex items-baseline gap-2">
      <dt className="shrink-0 text-text-disabled">{label}</dt>
      <dd className="break-all text-text-secondary">{value}</dd>
    </div>
  );
}

export default OntologyAboxPanel;

/**
 * OntologyAboxPanel browses the ABox of one scope: the instances a model
 * compiled (kind=entity) or the assertions it compiled (kind=relation), as
 * pages of rows, with the source text behind each row.
 *
 * It replaces the artifact lists that used to hang off the ontology model
 * graph. Those were driven by the graph's class selection and could only grow
 * their window ("load more"), which made "how much of this class have I seen"
 * unanswerable; rows here are engine-paged with a total, so the panel can say
 * both numbers.
 *
 * Editing is deliberately absent. A compiled row is an assertion about a
 * document; changing it in place would leave a claim whose source no longer
 * says what the row says. Judging a row (accept / reject) belongs to the
 * review queue, which owns the state machine.
 */
export function OntologyAboxPanel({
  datasetId,
  documentId,
  templateId,
  classes,
  properties,
  initialClass,
  initialProperty,
  initialSource,
  initialTarget,
  className,
}: OntologyAboxPanelProps) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<AboxRowKind>(
    initialProperty ? 'relation' : 'entity',
  );
  const [entityType, setEntityType] = useState(initialClass ?? '');
  const [property, setProperty] = useState(initialProperty ?? '');
  const [sourceType, setSourceType] = useState(initialSource ?? '');
  const [targetType, setTargetType] = useState(initialTarget ?? '');
  const [offset, setOffset] = useState(0);
  const [activeKey, setActiveKey] = useState('');
  const [evidenceOpen, setEvidenceOpen] = useState(false);

  // A filter change is a new list, not the next page of the old one: keeping
  // the offset would open the new filter on an empty page past its end.
  useEffect(() => {
    setOffset(0);
    setActiveKey('');
  }, [
    kind,
    entityType,
    property,
    sourceType,
    targetType,
    documentId,
    datasetId,
  ]);

  const enabled = !!datasetId;
  const query = useQuery({
    queryKey: [
      'ontology-abox',
      datasetId,
      documentId,
      templateId,
      kind,
      entityType,
      property,
      sourceType,
      targetType,
      offset,
    ],
    enabled,
    placeholderData: keepPreviousData,
    queryFn: () =>
      documentStructureService.getOntologyArtifacts(datasetId, {
        kind,
        class: entityType || undefined,
        property: property || undefined,
        source_type: sourceType || undefined,
        target_type: targetType || undefined,
        template_id: templateId,
        document_id: documentId,
        offset,
        limit: PageSize,
      }),
  });

  const page = query.data?.data?.data;
  const rows: (IStructureGraphEntity | IStructureGraphRelation)[] = useMemo(
    () =>
      kind === 'entity' ? (page?.entities ?? []) : (page?.relations ?? []),
    [kind, page],
  );
  const total = page?.total ?? 0;

  const activeIndex = rows.findIndex(
    (row, index) => rowKey(row, index) === activeKey,
  );
  const active =
    activeIndex >= 0
      ? rows[activeIndex]
      : (rows[0] as
          | IStructureGraphEntity
          | IStructureGraphRelation
          | undefined);

  // Declared vocabulary first (the closed set the model promises), then any
  // class the data carries but the model never declared — vocabulary drift is
  // something the reader has to be able to select and look at.
  const entityOptions = useMemo(() => {
    const seen = new Set((classes ?? []).map((item) => item.type));
    const observed = new Set<string>();
    rows.forEach((row) => {
      if (!isRelationRow(row) && row.type && !seen.has(row.type)) {
        observed.add(row.type);
      }
    });
    return [
      { label: t('knowledgeCompilation.aboxAllClasses'), value: '' },
      ...(classes ?? []).map((item) => ({
        label: item.label || item.type,
        value: item.type,
      })),
      ...Array.from(observed)
        .sort()
        .map((type) => ({ label: type, value: type })),
    ];
  }, [classes, rows, t]);

  const propertyOptions = useMemo(() => {
    const seen = new Set((properties ?? []).map((item) => item.type));
    const observed = new Set<string>();
    rows.forEach((row) => {
      if (isRelationRow(row) && row.type && !seen.has(row.type)) {
        observed.add(row.type);
      }
    });
    return [
      { label: t('knowledgeCompilation.aboxAllProperties'), value: '' },
      ...Array.from(new Set((properties ?? []).map((item) => item.type)))
        .sort()
        .map((type) => ({
          label: properties?.find((item) => item.type === type)?.label || type,
          value: type,
        })),
      ...Array.from(observed)
        .sort()
        .map((type) => ({ label: type, value: type })),
    ];
  }, [properties, rows, t]);

  const pickProperty = (name: string) => {
    setProperty(name);
    // A property edge is one domain → range combination. Selecting a declared
    // edge pins both sides, so the list is that edge's assertions and not every
    // combination the name appears in.
    const declared = (properties ?? []).find((item) => item.type === name);
    setSourceType(declared?.source ?? '');
    setTargetType(declared?.target ?? '');
  };

  const activeEntity =
    active && !isRelationRow(active)
      ? (active as IStructureGraphEntity)
      : undefined;
  const activeRelation = active && isRelationRow(active) ? active : undefined;
  const evidence = activeEntity?.evidence ?? activeRelation?.evidence ?? [];
  const chunkIds =
    activeEntity?.source_chunk_ids ?? activeRelation?.source_chunk_ids ?? [];
  const documentIds =
    activeEntity?.document_ids ?? activeRelation?.document_ids ?? [];

  const pageStart = total === 0 ? 0 : offset + 1;
  const pageEnd = Math.min(offset + rows.length, total);

  return (
    <div
      className={cn('flex h-full min-h-0 flex-col gap-2 text-xs', className)}
    >
      <header className="flex shrink-0 flex-wrap items-center gap-2">
        <div className="flex gap-1">
          <Button
            variant={kind === 'entity' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setKind('entity')}
          >
            {t('knowledgeCompilation.ontologyInstancesEntities')}
          </Button>
          <Button
            variant={kind === 'relation' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setKind('relation')}
          >
            {t('knowledgeCompilation.ontologyInstancesRelations')}
          </Button>
        </div>
        <div className="w-48">
          <SelectWithSearch
            value={kind === 'entity' ? entityType : property}
            options={kind === 'entity' ? entityOptions : propertyOptions}
            onChange={kind === 'entity' ? setEntityType : pickProperty}
            allowClear
          />
        </div>
        <span className="ml-auto text-text-secondary">
          {t('knowledgeCompilation.aboxPageRange', {
            start: pageStart,
            end: pageEnd,
            total,
            defaultValue: '{{start}}–{{end}} of {{total}}',
          })}
        </span>
      </header>

      <div className="flex min-h-0 flex-1 gap-2">
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-1 overflow-auto">
          {!enabled && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.aboxNoDataset', {
                defaultValue: 'Pick a dataset to read its compiled rows.',
              })}
            </span>
          )}
          {query.isFetching && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyDrilldownLoading')}
            </span>
          )}
          {query.isError && (
            <span className="text-[#F0A020]">
              {t('knowledgeCompilation.ontologyDrilldownFailed', {
                message:
                  (query.error as Error | null)?.message ?? 'unknown error',
              })}
            </span>
          )}
          {!query.isFetching &&
            !query.isError &&
            enabled &&
            rows.length === 0 && (
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyInstancesEmpty')}
              </span>
            )}
          {rows.map((row, index) => {
            const key = rowKey(row, index);
            const entity = isRelationRow(row) ? undefined : row;
            const relation = isRelationRow(row) ? row : undefined;
            return (
              <button
                key={key}
                type="button"
                onClick={() => setActiveKey(key)}
                className={cn(
                  'flex flex-col gap-1 rounded border p-2 text-start',
                  activeIndex === index || (activeIndex < 0 && index === 0)
                    ? 'border-text-accent'
                    : 'border-border-button',
                )}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">
                    {entity?.name ??
                      (relation && `${relation.from} → ${relation.to}`)}
                  </span>
                  <Badge variant="secondary">
                    {entity?.type ?? relation?.type ?? ''}
                  </Badge>
                  <ReviewBadge status={row.review_status} />
                  {typeof row.version === 'number' && (
                    <span className="text-text-disabled">v{row.version}</span>
                  )}
                </div>
                {(entity?.description ?? relation?.description) && (
                  <p className="line-clamp-2 text-text-secondary">
                    {entity?.description ?? relation?.description}
                  </p>
                )}
              </button>
            );
          })}
        </div>

        <aside className="flex w-72 shrink-0 flex-col gap-2 overflow-auto rounded border border-border-button p-2">
          {active ? (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">
                  {activeEntity?.name ??
                    (activeRelation &&
                      `${activeRelation.from} → ${activeRelation.to}`)}
                </span>
                <span className="text-text-secondary">
                  {activeEntity?.type ?? activeRelation?.type ?? ''}
                </span>
                <ReviewBadge status={active.review_status} />
              </div>

              <dl className="flex flex-col gap-1">
                <DetailRow label="scope" value={active.scope ?? ''} />
                <DetailRow
                  label="version"
                  value={
                    typeof active.version === 'number'
                      ? String(active.version)
                      : ''
                  }
                />
                <DetailRow
                  label="mentions"
                  value={
                    activeEntity?.mention_count
                      ? String(activeEntity.mention_count)
                      : ''
                  }
                />
                <DetailRow
                  label="aliases"
                  value={activeEntity?.aliases?.join(', ') ?? ''}
                />
                <DetailRow
                  label="edge"
                  value={
                    activeRelation
                      ? `${activeRelation.source_type ?? '?'} → ${
                          activeRelation.target_type ?? '?'
                        }`
                      : ''
                  }
                />
                <DetailRow label="reviewed by" value={active.review_by ?? ''} />
                <DetailRow label="note" value={active.review_note ?? ''} />
              </dl>

              {activeEntity && attributeEntries(activeEntity).length > 0 && (
                <section className="flex flex-col gap-1">
                  <span className="text-text-secondary">
                    {t('knowledgeCompilation.ontologyAttributes')}
                  </span>
                  {attributeEntries(activeEntity).map(([name, value]) => (
                    <div key={name} className="flex gap-1">
                      <span className="shrink-0 text-text-disabled">
                        {name}
                      </span>
                      <span className="break-all text-text-primary">
                        {formatAttributeValue(value)}
                      </span>
                    </div>
                  ))}
                </section>
              )}

              {evidence.length > 0 || chunkIds.length > 0 ? (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setEvidenceOpen(true)}
                  >
                    {evidence.length > 0
                      ? t('knowledgeCompilation.aboxSourceQuotes', {
                          count: evidence.length,
                          defaultValue: 'Source quotes ({{count}})',
                        })
                      : t('knowledgeCompilation.aboxSourceText', {
                          count: chunkIds.length,
                          defaultValue: 'Source text ({{count}})',
                        })}
                  </Button>
                  <OntologyEvidenceView
                    open={evidenceOpen}
                    datasetId={datasetId}
                    documentIds={documentIds}
                    chunkIds={chunkIds}
                    evidence={evidence}
                  />
                </>
              ) : (
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyInstancesNoEvidence')}
                </span>
              )}
            </>
          ) : (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInstancesPickOne')}
            </span>
          )}
        </aside>
      </div>

      <footer className="flex shrink-0 items-center justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={offset === 0 || query.isFetching}
          onClick={() => setOffset((value) => Math.max(0, value - PageSize))}
        >
          {t('knowledgeCompilation.aboxPrevious', { defaultValue: 'Previous' })}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={pageEnd >= total || query.isFetching}
          onClick={() => setOffset((value) => value + PageSize)}
        >
          {t('knowledgeCompilation.aboxNext', { defaultValue: 'Next' })}
        </Button>
      </footer>
    </div>
  );
}
