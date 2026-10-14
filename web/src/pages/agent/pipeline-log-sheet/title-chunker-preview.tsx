import { cn } from '@/lib/utils';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

type UnknownRecord = Record<string, unknown>;

type PreviewProps = {
  value: unknown;
};

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function asRecords(value: unknown): UnknownRecord[] {
  return Array.isArray(value) ? value.filter(isRecord) : [];
}

function asStrings(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : [];
}

function formatRecordIndexes(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is number => typeof item === 'number').join(', ')
    : '';
}

function MatchInfo({ match }: { match: unknown }) {
  const { t } = useTranslation();
  if (!isRecord(match)) return null;

  const source =
    match.source === 'frequency'
      ? t('flow.titleChunkerPreview.sourceRegex', 'Regex')
      : match.source === 'outline'
        ? t('flow.titleChunkerPreview.sourceOutline', 'PDF outline')
        : match.source === 'layout'
          ? t('flow.titleChunkerPreview.sourceLayout', 'Layout')
          : '';
  const pattern = typeof match.pattern === 'string' ? match.pattern : '';
  const ruleGroup =
    typeof match.rule_group_index === 'number'
      ? t('flow.titleChunkerPreview.ruleGroup', { index: match.rule_group_index + 1, defaultValue: `Rule group ${match.rule_group_index + 1}` })
      : '';
  const rule =
    typeof match.rule_index === 'number'
      ? t('flow.titleChunkerPreview.rule', { index: match.rule_index + 1, defaultValue: `Rule ${match.rule_index + 1}` })
      : '';
  const details = [
    source,
    ruleGroup,
    rule,
    pattern ? `/${pattern}/` : '',
  ].filter(Boolean);

  return details.length > 0 ? (
    <span className="text-xs text-text-secondary">{details.join(' · ')}</span>
  ) : null;
}

function HeadingList({ headings }: { headings: UnknownRecord[] }) {
  const { t } = useTranslation();
  if (headings.length === 0) {
    return (
      <p className="text-sm text-text-secondary">
        {t('flow.titleChunkerPreview.noHeadings', 'No headings detected.')}
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {headings.map((heading, index) => {
        const level = typeof heading.level === 'number' ? heading.level : '?';
        const text = typeof heading.text === 'string' ? heading.text : '(untitled)';
        return (
          <div className="flex items-start gap-3 rounded border px-3 py-2" key={`${String(heading.record_index)}-${index}`}>
            <span className="shrink-0 font-medium text-accent-primary">H{level}</span>
            <div className="min-w-0">
              <div className="break-words text-sm">{text}</div>
              <MatchInfo match={heading.match} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

function HierarchyNode({ node, depth = 0 }: { node: UnknownRecord; depth?: number }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const handleToggle = () => setOpen((current) => !current);
  const children = asRecords(node.children);
  const titles = formatRecordIndexes(node.title_record_indexes);
  const bodies = formatRecordIndexes(node.body_record_indexes);
  const level = typeof node.level === 'number' ? node.level : 0;

  return (
    <div className="space-y-1" style={{ marginLeft: depth * 16 }}>
      <button
        className="flex w-full items-start gap-1 text-left text-sm"
        onClick={handleToggle}
        type="button"
      >
        {children.length > 0 ? (
          open ? <ChevronDown className="mt-0.5 size-4 shrink-0" /> : <ChevronRight className="mt-0.5 size-4 shrink-0" />
        ) : (
          <span className="size-4 shrink-0" />
        )}
        <span className="font-medium">H{level}</span>
        <span className="text-xs text-text-secondary">
          {titles
            ? t('flow.titleChunkerPreview.headingRecords', { indexes: titles, defaultValue: `Heading records: ${titles}` })
            : ''}
          {bodies
            ? ` · ${t('flow.titleChunkerPreview.bodyRecords', { indexes: bodies, defaultValue: `Body records: ${bodies}` })}`
            : ''}
        </span>
      </button>
      {open && children.map((child, index) => <HierarchyNode key={index} depth={depth + 1} node={child} />)}
    </div>
  );
}

function ChunkList({ chunks, sectionIds }: { chunks: UnknownRecord[]; sectionIds?: unknown }) {
  const { t } = useTranslation();
  if (chunks.length === 0) {
    return (
      <p className="text-sm text-text-secondary">
        {t('flow.titleChunkerPreview.noChunks', 'No chunks generated.')}
      </p>
    );
  }
  const ids = Array.isArray(sectionIds) ? sectionIds : [];

  return (
    <div className="space-y-3">
      {chunks.map((chunk, index) => {
        const path = asStrings(chunk.section_path);
        const content = typeof chunk.content === 'string' ? chunk.content : '';
        return (
          <div className="rounded border p-3" key={index}>
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
              <span>
                {t('flow.titleChunkerPreview.chunk', { index: index + 1, defaultValue: `Chunk ${index + 1}` })}
              </span>
              <span>
                {t('flow.titleChunkerPreview.records', {
                  indexes: formatRecordIndexes(chunk.record_indexes) || t('flow.titleChunkerPreview.noRecords', 'none'),
                  defaultValue: `Records: ${formatRecordIndexes(chunk.record_indexes) || 'none'}`,
                })}
              </span>
              {ids[index] !== undefined && (
                <span>
                  {t('flow.titleChunkerPreview.section', {
                    id: String(ids[index]),
                    defaultValue: `Section: ${String(ids[index])}`,
                  })}
                </span>
              )}
            </div>
            {path.length > 0 && <div className="mt-1 text-sm font-medium">{path.join(' > ')}</div>}
            <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-words text-sm">{content}</pre>
          </div>
        );
      })}
    </div>
  );
}

export function TitleChunkerPreview({ value }: PreviewProps) {
  const { t } = useTranslation();
  if (!isRecord(value)) return null;
  const mode = value.mode === 'hierarchy' || value.mode === 'group' ? value.mode : null;
  if (!mode) return null;

  const headings = asRecords(value.headings);
  const chunks = asRecords(value.chunks);
  const hierarchy = asRecords(value.hierarchy);
  const sectionIds = value.section_ids;
  const modeLabel = t(
    mode === 'hierarchy'
      ? 'flow.titleChunkerPreview.hierarchyMode'
      : 'flow.titleChunkerPreview.groupMode',
    mode === 'hierarchy' ? 'Hierarchy' : 'Group',
  );

  return (
    <section className="mb-5 space-y-4 rounded-lg border border-accent-primary/40 bg-accent-primary/5 p-4">
      <div>
        <h2 className="text-base font-semibold">
          {t('flow.titleChunkerPreview.title', 'Title Chunker preview')}
        </h2>
        <p className="text-xs text-text-secondary">
          {t('flow.titleChunkerPreview.mode', { mode: modeLabel, defaultValue: `Mode: ${modeLabel}` })}
        </p>
      </div>

      <div>
        <h3 className="mb-2 text-sm font-medium">
          {t('flow.titleChunkerPreview.headings', 'Headings')}
        </h3>
        <HeadingList headings={headings} />
      </div>

      {mode === 'hierarchy' && hierarchy.length > 0 && (
        <div>
          <h3 className="mb-2 text-sm font-medium">
            {t('flow.titleChunkerPreview.hierarchy', 'Hierarchy')}
          </h3>
          <div className={cn('space-y-2 rounded border p-3')}>
            {hierarchy.map((node, index) => <HierarchyNode key={index} node={node} />)}
          </div>
        </div>
      )}

      <div>
        <h3 className="mb-2 text-sm font-medium">
          {t('flow.titleChunkerPreview.generatedChunks', 'Generated chunks')}
        </h3>
        <ChunkList chunks={chunks} sectionIds={mode === 'group' ? sectionIds : undefined} />
      </div>
    </section>
  );
}
