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
  IOntologyEntityPage,
  IOntologyRelationPage,
  IStructureGraphEntity,
} from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import documentStructureService from '@/services/document-structure-service';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

/** One object-property edge as the panel lists it for a class. */
export interface OntologyDetailProperty {
  type: string;
  label: string;
  source: string;
  target: string;
  relations: number;
  declared: boolean;
}

/**
 * A class referenced from the panel. Both halves travel together because the
 * panel PRINTS the label but must SELECT by the name — the two differ whenever
 * the template declares a label, and a navigation that used the label would find
 * nothing in the graph.
 */
export interface OntologyDetailClassRef {
  name: string;
  label: string;
}

/**
 * Everything the class panel shows. The graph resolves it (it owns the nodes and
 * the edges), so the panel stays a renderer and never re-derives the ontology.
 */
export interface OntologyDetailSelection {
  /** The class NAME — the identifier filters and drill-down URLs are keyed by. */
  className: string;
  /** What gets printed; equals className when the template declares no label. */
  label: string;
  description?: string;
  entities: number;
  declared: boolean;
  /** Superclasses, nearest first. */
  parents: OntologyDetailClassRef[];
  /** Subclasses. */
  children: OntologyDetailClassRef[];
  /** Effective datatype properties: own + inherited, with their own counts. */
  attributes: IOntologyAttribute[];
  /** Object-property edges touching this class. */
  properties: OntologyDetailProperty[];
}

/**
 * The published state. `datasetId` / `documentId` / `templateId` scope the
 * drill-down; without a dataset the panel still lists the ontology, it just
 * cannot page the rows.
 *
 * `documentId` matters on a document page: the graph's counts there are that
 * document's, and a drill-down that ignored it would list rows from the whole
 * dataset under a count that came from one document.
 */
export interface OntologyDetailPanelState extends OntologyDetailSelection {
  /**
   * The properties of the edge that was clicked (a bundle holds several). The
   * rows they name are marked, so "what is this edge?" has an answer on screen.
   */
  focusedProperties?: string[];
  datasetId?: string;
  documentId?: string;
  templateId?: string;
  /** Drill a class or one of its property edges — resolves that class/edge. */
  onSelect?: (className: string) => void;
  onClose: () => void;
}

/** One drill-down page. The panel shows a list, not a dump. */
const DrilldownPageSize = 20;

/**
 * The most rows one drill-down may ask for in total. It mirrors the service's
 * own cap (`ontologyDrilldownMaxLimit`) on purpose: without it "Load more" would
 * keep growing the request while the server silently stopped adding rows, and
 * the panel would look stuck rather than finished at its limit.
 */
const DrilldownMaxLimit = 200;

/**
 * How many attribute values an instance row previews. A row is a line in a list,
 * not a table: the point is to show that the value exists and what it looks
 * like, and the class panel's own attribute list already says how many
 * assertions there are in total.
 */
const AttributePreviewCount = 3;

type DrilldownTarget =
  | { kind: 'class'; name: string }
  | { kind: 'property'; name: string; source: string; target: string };

/**
 * attributeEntries reads an instance's datatype attributes as [name, value]
 * pairs, dropping the empty ones: the engine omits a property the model could
 * not fill sometimes and stores an empty string other times, and neither should
 * take one of the preview slots.
 */
const attributeEntries = (entity: IStructureGraphEntity): [string, unknown][] =>
  Object.entries(entity.attributes ?? {}).filter(
    ([, value]) => value !== null && value !== undefined && value !== '',
  );

/** formatAttributeValue prints a JSON attribute value the way a reader reads it. */
const formatAttributeValue = (value: unknown): string => {
  if (Array.isArray(value)) {
    return value.map((item) => formatAttributeValue(item)).join(', ');
  }
  if (value !== null && typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
};

/**
 * OntologyDetailPanel renders the selected class.
 *
 * It is a separate component from the canvas on purpose: the canvas lives inside
 * the artifact column, and a panel drawn there is squashed (or covered) as soon
 * as the column divider moves — the divider only knows about the page's panels.
 * The page renders this one as its middle column instead, so the divider obeys
 * it like any other panel, and the canvas keeps its full width.
 */
export function OntologyDetailPanel({
  className: classType,
  label,
  description,
  entities,
  declared,
  parents,
  children,
  attributes,
  properties,
  focusedProperties,
  datasetId,
  documentId,
  templateId,
  onSelect,
  onClose,
}: OntologyDetailPanelState) {
  const { t } = useTranslation();
  const [drilldown, setDrilldown] = useState<DrilldownTarget | null>(null);
  // One page is what fits a panel; "Load more" grows the window instead of
  // paging with next/prev, because the rows are read top-down and a reader does
  // not want to lose the first page to see the second.
  const [drilldownLimit, setDrilldownLimit] = useState(DrilldownPageSize);

  // A drill-down belongs to the class it was opened from: switching classes in
  // the canvas must not leave the previous class's rows under the new heading.
  // The window resets with it, or a grown page would leak into the next target.
  useEffect(() => {
    setDrilldown(null);
    setDrilldownLimit(DrilldownPageSize);
  }, [classType]);

  const drilldownQuery = useQuery({
    queryKey: [
      'ontology-drilldown',
      datasetId,
      documentId,
      templateId,
      drilldown,
      drilldownLimit,
    ],
    enabled: !!datasetId && !!drilldown,
    queryFn: async (): Promise<
      IOntologyEntityPage | IOntologyRelationPage | null
    > => {
      if (!datasetId || !drilldown) return null;
      if (drilldown.kind === 'class') {
        const { data } =
          await documentStructureService.getOntologyClassEntities(
            datasetId,
            drilldown.name,
            {
              template_id: templateId,
              // Both halves of the scope. On a document page the graph's counts
              // are that document's, so a list without `document_id` would be
              // counting one scope and listing another.
              document_id: documentId,
              limit: drilldownLimit,
            },
          );
        return data?.data ?? null;
      }
      const { data } =
        await documentStructureService.getOntologyPropertyRelations(
          datasetId,
          drilldown.name,
          {
            source_type: drilldown.source,
            target_type: drilldown.target,
            template_id: templateId,
            document_id: documentId,
            limit: drilldownLimit,
          },
        );
      return data?.data ?? null;
    },
  });

  const drilldownEntities =
    drilldownQuery.data && 'entities' in drilldownQuery.data
      ? drilldownQuery.data.entities
      : [];
  const drilldownRelations =
    drilldownQuery.data && 'relations' in drilldownQuery.data
      ? drilldownQuery.data.relations
      : [];
  const drilldownTotal = drilldownQuery.data?.total ?? 0;
  const drilldownShown = drilldownEntities.length + drilldownRelations.length;

  // The drill-down REPLACES the class view instead of being appended to it. It
  // used to render below the Attributes / Properties / Inheritance sections, so
  // on a long panel the list landed under the fold and pressing "View instances"
  // read as "nothing happened". A view of its own with a way back is also what
  // the button's wording promises.
  if (drilldown) {
    return (
      <div className="flex h-full min-h-0 flex-col gap-2 overflow-auto p-5 text-xs">
        <div className="flex items-start justify-between gap-2">
          <button
            type="button"
            className="min-w-0 truncate text-left text-text-secondary hover:text-text-primary"
            onClick={() => setDrilldown(null)}
          >
            ← {label}
          </button>
          <button
            type="button"
            className="shrink-0 text-text-secondary hover:text-text-primary"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        <section className="flex flex-col gap-1">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate font-medium">{drilldown.name}</span>
          </div>
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyDrilldownTotal', {
              count: drilldownTotal,
              defaultValue: '{{count}} row(s) in scope',
            })}
          </span>
          {drilldownQuery.isFetching && (
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyDrilldownLoading', {
                defaultValue: 'Loading…',
              })}
            </span>
          )}
          {/* A failed request MUST be visible: the drill-down hits an endpoint
              the other views never touch, so a 404 (an older backend without the
              route) would otherwise look exactly like "this scope is empty". */}
          {drilldownQuery.isError && (
            <span className="text-[#F0A020]">
              {t('knowledgeCompilation.ontologyDrilldownFailed', {
                message:
                  (drilldownQuery.error as Error | null)?.message ??
                  'unknown error',
                defaultValue: 'The request failed: {{message}}',
              })}
            </span>
          )}
          {!drilldownQuery.isFetching &&
            !drilldownQuery.isError &&
            drilldownTotal === 0 && (
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyDrilldownEmpty', {
                  defaultValue: 'Nothing compiled for this yet.',
                })}
              </span>
            )}
          {drilldownEntities.map((entity) => (
            <div
              key={entity.id ?? entity.name}
              className="rounded border border-border-button bg-bg-card px-2 py-1"
            >
              <div className="truncate font-medium">{entity.name}</div>
              {(entity.description || entity.discription) && (
                <div className="line-clamp-2 text-text-secondary">
                  {entity.description || entity.discription}
                </div>
              )}
              {/* The ontology's own data: without this an instance list would be
                  names only, and the attribute VALUES — the thing a reader
                  actually came for — would be visible nowhere in the UI. */}
              {attributeEntries(entity).length > 0 && (
                <div className="mt-0.5 flex flex-col gap-0.5 text-text-secondary">
                  {attributeEntries(entity)
                    .slice(0, AttributePreviewCount)
                    .map(([name, value]) => (
                      <div key={name} className="flex gap-1">
                        <span className="shrink-0">{name}</span>
                        <span className="truncate text-text-primary">
                          {formatAttributeValue(value)}
                        </span>
                      </div>
                    ))}
                  {attributeEntries(entity).length > AttributePreviewCount && (
                    <span>
                      {t('knowledgeCompilation.ontologyMoreAttributes', {
                        count:
                          attributeEntries(entity).length -
                          AttributePreviewCount,
                        defaultValue: '+{{count}} more attribute(s)',
                      })}
                    </span>
                  )}
                </div>
              )}
            </div>
          ))}
          {drilldownRelations.map((relation, index) => (
            <div
              key={`${relation.from}:${relation.to}:${index}`}
              className="rounded border border-border-button bg-bg-card px-2 py-1"
            >
              <div className="truncate">
                <span className="font-medium">{relation.from}</span>
                <span className="text-text-secondary">
                  {' → '}
                  {relation.to}
                </span>
              </div>
              {relation.type && (
                <div className="text-text-secondary">{relation.type}</div>
              )}
            </div>
          ))}
          {drilldownShown < drilldownTotal &&
            (drilldownLimit < DrilldownMaxLimit ? (
              // A list that silently stops is worse than a short one: the count
              // above says how many exist, this says how many are on screen, and
              // the button is how the rest arrive (the engines page, they do not
              // stream).
              <div className="flex items-center justify-between gap-2 pt-1">
                <span className="text-text-secondary">
                  {t('knowledgeCompilation.ontologyDrilldownMore', {
                    count: drilldownTotal - drilldownShown,
                    defaultValue: '{{count}} more not shown',
                  })}
                </span>
                <button
                  type="button"
                  className="shrink-0 rounded border border-border-button px-2 py-0.5 hover:bg-bg-base disabled:opacity-50"
                  disabled={drilldownQuery.isFetching}
                  onClick={() =>
                    setDrilldownLimit((limit) => limit + DrilldownPageSize)
                  }
                >
                  {t('knowledgeCompilation.ontologyDrilldownLoadMore', {
                    count: Math.min(
                      DrilldownPageSize,
                      drilldownTotal - drilldownShown,
                    ),
                    defaultValue: 'Load {{count}} more',
                  })}
                </button>
              </div>
            ) : (
              // At the cap the button would lie (the service caps the request
              // too), so the panel states the limit instead of pretending more
              // is available.
              <span className="text-text-secondary">
                {t('knowledgeCompilation.ontologyDrilldownCapped', {
                  shown: drilldownShown,
                  total: drilldownTotal,
                  defaultValue:
                    'Showing the first {{shown}} of {{total}} — open the class in a narrower scope to see the rest.',
                })}
              </span>
            ))}
          {drilldownShown > 0 && drilldownShown >= drilldownTotal && (
            // Everything is on screen: say so, so the reader knows the list is
            // complete rather than wondering where the button went.
            <span className="text-text-secondary">
              {t('knowledgeCompilation.ontologyDrilldownAllShown', {
                count: drilldownTotal,
                defaultValue: 'All {{count}} row(s) shown',
              })}
            </span>
          )}
        </section>
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-2 overflow-auto p-5 text-xs">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 flex-col">
          <span className="truncate text-sm font-medium">{label}</span>
          {label !== classType && (
            // The name is what filters and the drill-down use, so it stays
            // visible whenever it differs from what the canvas prints.
            <span className="truncate text-text-secondary">{classType}</span>
          )}
        </div>
        <button
          type="button"
          className="shrink-0 text-text-secondary hover:text-text-primary"
          onClick={onClose}
        >
          ×
        </button>
      </div>

      {!declared && (
        <div className="rounded border border-[#F0A020] px-2 py-1 text-[#F0A020]">
          {t('knowledgeCompilation.ontologyUndeclaredClass', {
            defaultValue: 'Observed, not declared',
          })}
        </div>
      )}
      {description && <p className="text-text-secondary">{description}</p>}

      <div className="flex items-center justify-between gap-2">
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyInstanceCount', {
            count: entities,
            defaultValue: '{{count}} instance(s)',
          })}
        </span>
        {/* The button is only rendered when there is something to open. A
            disabled button is a dead end: it swallows the click, prints nothing
            and fires no request, so the reader cannot tell "0 instances" from
            "the view is broken". Saying it in words is honest and actionable. */}
        {datasetId && entities > 0 && (
          <button
            type="button"
            className="shrink-0 rounded border border-border-button px-2 py-0.5 hover:bg-bg-base"
            onClick={() => setDrilldown({ kind: 'class', name: classType })}
          >
            {t('knowledgeCompilation.ontologyViewInstances', {
              defaultValue: 'View instances',
            })}
          </button>
        )}
      </div>
      {datasetId && entities === 0 && (
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyNoInstancesToShow', {
            defaultValue:
              'No instance of this class was compiled in this scope, so there is no list to open.',
          })}
        </span>
      )}

      {/* Datatype properties: they have no class as their range, so they are not
          edges — they are the attributes of an instance, and this is where a
          reader looks for them. */}
      <section className="flex flex-col gap-1">
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyAttributes', {
            defaultValue: 'Attributes',
          })}
        </span>
        {attributes.length === 0 && (
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyNoAttributes', {
              defaultValue: 'No datatype property is declared for it.',
            })}
          </span>
        )}
        {attributes.map((attribute) => (
          <div
            key={attribute.type}
            className="rounded border border-border-button px-2 py-1"
          >
            <div className="flex items-center justify-between gap-2">
              <span className="truncate font-medium">
                {attribute.label || attribute.type}
              </span>
              <span className="shrink-0 text-text-secondary">
                {attribute.assertions}
              </span>
            </div>
            <div className="flex flex-wrap items-center gap-1 text-text-secondary">
              {attribute.datatype && <span>{attribute.datatype}</span>}
              {attribute.inherited_from && (
                <span>
                  {t('knowledgeCompilation.ontologyInheritedFrom', {
                    name: attribute.inherited_from,
                    defaultValue: 'inherited from {{name}}',
                  })}
                </span>
              )}
            </div>
          </div>
        ))}
      </section>

      <section className="flex flex-col gap-1">
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyProperties', {
            defaultValue: 'Properties',
          })}
        </span>
        {properties.length === 0 && (
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyNoProperties', {
              defaultValue: 'No property touches this class.',
            })}
          </span>
        )}
        {properties.map((property) => {
          const outgoing = property.source === classType;
          return (
            <div
              key={`${property.type}:${property.source}:${property.target}`}
              className={cn(
                'flex flex-col gap-0.5 rounded border px-2 py-1',
                property.declared ? 'border-border-button' : 'border-[#F0A020]',
                // The bundle the reader clicked, marked: this is the answer to
                // "what is this edge?" on a panel that lists a whole class.
                focusedProperties?.includes(property.type) &&
                  'ring-1 ring-[#4CACFF]',
              )}
            >
              <div className="flex items-center gap-1">
                <span className="truncate font-medium">
                  {property.label || property.type}
                </span>
                <span className="ml-auto shrink-0 text-text-secondary">
                  {outgoing
                    ? `${property.source} → ${property.target}`
                    : `${property.target} ← ${property.source}`}
                </span>
                <span className="shrink-0 text-text-secondary">
                  {property.relations}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  className="self-start rounded border border-border-button px-2 py-0.5 hover:bg-bg-base"
                  onClick={() =>
                    onSelect?.(outgoing ? property.target : property.source)
                  }
                >
                  {t('knowledgeCompilation.ontologyGoToClass', {
                    defaultValue: 'Go to the other class',
                  })}
                </button>
                {/* Same rule as "View instances": an edge with no assertion
                    gets no button, because there is nothing behind it. The 0
                    printed above already says the count. */}
                {datasetId && property.relations > 0 && (
                  <button
                    type="button"
                    className="rounded border border-border-button px-2 py-0.5 hover:bg-bg-base"
                    onClick={() =>
                      setDrilldown({
                        kind: 'property',
                        name: property.type,
                        source: property.source,
                        target: property.target,
                      })
                    }
                  >
                    {t('knowledgeCompilation.ontologyViewAssertions', {
                      defaultValue: 'View assertions',
                    })}
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </section>

      {/* Inheritance is kept apart from the properties: it is a schema statement
          with no count, so it cannot be ranked or drilled into. */}
      <section className="flex flex-col gap-1">
        <span className="text-text-secondary">
          {t('knowledgeCompilation.ontologyInheritance', {
            defaultValue: 'Inheritance',
          })}
        </span>
        {parents.length === 0 && children.length === 0 && (
          <span className="text-text-secondary">
            {t('knowledgeCompilation.ontologyNoInheritance', {
              defaultValue: 'No superclass and no subclass.',
            })}
          </span>
        )}
        {parents.map((parent) => (
          <button
            key={`parent:${parent.name}`}
            type="button"
            className="truncate rounded border border-[#8B5CF6] px-2 py-1 text-left hover:bg-bg-base"
            onClick={() => onSelect?.(parent.name)}
          >
            <span className="text-text-secondary">↑ </span>
            {parent.label}
          </button>
        ))}
        {children.map((child) => (
          <button
            key={`child:${child.name}`}
            type="button"
            className="truncate rounded border border-[#8B5CF6] px-2 py-1 text-left hover:bg-bg-base"
            onClick={() => onSelect?.(child.name)}
          >
            <span className="text-text-secondary">↓ </span>
            {child.label}
          </button>
        ))}
      </section>
    </div>
  );
}

export default OntologyDetailPanel;
