import { cn } from '@/lib/utils';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { useState } from 'react';

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
  if (!isRecord(match)) return null;

  const source = typeof match.source === 'string' ? match.source : '';
  const pattern = typeof match.pattern === 'string' ? match.pattern : '';
  const ruleGroup = typeof match.rule_group_index === 'number' ? match.rule_group_index + 1 : null;
  const rule = typeof match.rule_index === 'number' ? match.rule_index + 1 : null;
  const details = [
    source,
    ruleGroup === null ? '' : `rule group ${ruleGroup}`,
    rule === null ? '' : `rule ${rule}`,
    pattern ? `/${pattern}/` : '',
  ].filter(Boolean);

  return details.length > 0 ? (
    <span className="text-xs text-text-secondary">{details.join(' · ')}</span>
  ) : null;
}

function HeadingList({ headings }: { headings: UnknownRecord[] }) {
  if (headings.length === 0) return <p className="text-sm text-text-secondary">No headings detected.</p>;

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
  const [open, setOpen] = useState(true);
  const children = asRecords(node.children);
  const titles = formatRecordIndexes(node.title_record_indexes);
  const bodies = formatRecordIndexes(node.body_record_indexes);
  const level = typeof node.level === 'number' ? node.level : 0;

  return (
    <div className="space-y-1" style={{ marginLeft: depth * 16 }}>
      <button
        className="flex w-full items-start gap-1 text-left text-sm"
        onClick={() => setOpen((current) => !current)}
        type="button"
      >
        {children.length > 0 ? (
          open ? <ChevronDown className="mt-0.5 size-4 shrink-0" /> : <ChevronRight className="mt-0.5 size-4 shrink-0" />
        ) : (
          <span className="size-4 shrink-0" />
        )}
        <span className="font-medium">H{level}</span>
        <span className="text-xs text-text-secondary">
          {titles ? `heading records: ${titles}` : ''}
          {bodies ? ` · body records: ${bodies}` : ''}
        </span>
      </button>
      {open && children.map((child, index) => <HierarchyNode key={index} depth={depth + 1} node={child} />)}
    </div>
  );
}

function ChunkList({ chunks, sectionIds }: { chunks: UnknownRecord[]; sectionIds?: unknown }) {
  if (chunks.length === 0) return <p className="text-sm text-text-secondary">No chunks generated.</p>;
  const ids = Array.isArray(sectionIds) ? sectionIds : [];

  return (
    <div className="space-y-3">
      {chunks.map((chunk, index) => {
        const path = asStrings(chunk.section_path);
        const content = typeof chunk.content === 'string' ? chunk.content : '';
        return (
          <div className="rounded border p-3" key={index}>
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
              <span>Chunk {index + 1}</span>
              <span>Records: {formatRecordIndexes(chunk.record_indexes) || 'none'}</span>
              {ids[index] !== undefined && <span>Section: {String(ids[index])}</span>}
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
  if (!isRecord(value)) return null;
  const mode = value.mode === 'hierarchy' || value.mode === 'group' ? value.mode : null;
  if (!mode) return null;

  const headings = asRecords(value.headings);
  const chunks = asRecords(value.chunks);
  const hierarchy = asRecords(value.hierarchy);
  const sectionIds = value.section_ids;

  return (
    <section className="mb-5 space-y-4 rounded-lg border border-accent-primary/40 bg-accent-primary/5 p-4">
      <div>
        <h2 className="text-base font-semibold">Title Chunker preview</h2>
        <p className="text-xs text-text-secondary">Mode: {mode}</p>
      </div>

      <div>
        <h3 className="mb-2 text-sm font-medium">Headings</h3>
        <HeadingList headings={headings} />
      </div>

      {mode === 'hierarchy' && hierarchy.length > 0 && (
        <div>
          <h3 className="mb-2 text-sm font-medium">Hierarchy</h3>
          <div className={cn('space-y-2 rounded border p-3')}>
            {hierarchy.map((node, index) => <HierarchyNode key={index} node={node} />)}
          </div>
        </div>
      )}

      <div>
        <h3 className="mb-2 text-sm font-medium">Generated chunks</h3>
        <ChunkList chunks={chunks} sectionIds={mode === 'group' ? sectionIds : undefined} />
      </div>
    </section>
  );
}
