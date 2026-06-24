import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  useFetchDatasetSkillPage,
  useFetchDatasetSkillTree,
} from '@/hooks/use-dataset-skill-request';
import { DatasetSkillTreeNode } from '@/interfaces/database/dataset-skill';
import { cn } from '@/lib/utils';
import {
  ChevronDown,
  ChevronRight,
  FileText,
  Folder,
  FolderOpen,
  Sparkles,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import { useSearchParams } from 'react-router';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';

type SkillTreeNodeProps = {
  node: DatasetSkillTreeNode;
  depth: number;
  selectedSkill: string | null;
  onSelect: (skillKwd: string) => void;
};

function firstSkill(nodes: DatasetSkillTreeNode[] | undefined): string | null {
  if (!nodes?.length) return null;
  return nodes[0].skill_kwd || null;
}

function frontmatterValue(md: string | undefined, key: string): string {
  if (!md) return '';
  const lines = md.split(/\r?\n/);
  const index = lines.findIndex((line) => line.trim() === `${key}: >`);
  if (index >= 0) {
    const block: string[] = [];
    for (let i = index + 1; i < lines.length; i += 1) {
      const line = lines[i];
      if (!line.startsWith('  ')) break;
      block.push(line.trim());
    }
    return block.join(' ').trim();
  }

  const scalar = lines.find((line) => line.startsWith(`${key}:`));
  return scalar?.slice(key.length + 1).trim() ?? '';
}

function stripFrontmatter(md: string): string {
  const lines = md.split(/\r?\n/);
  if (lines[0]?.trim() !== '---') return md;
  const end = lines.slice(1).findIndex((line) => line.trim() === '---');
  return end >= 0
    ? lines
        .slice(end + 2)
        .join('\n')
        .trimStart()
    : md;
}

function skillLabel(skillKwd: string): string {
  return skillKwd || 'skill';
}

function tooltipSnippet(node: DatasetSkillTreeNode): string {
  const desc = frontmatterValue(node.md_with_weight, 'description');
  return (
    desc || node.md_with_weight?.replace(/^---[\s\S]*?---/, '').trim() || ''
  );
}

function SkillTreeNode({
  node,
  depth,
  selectedSkill,
  onSelect,
}: SkillTreeNodeProps) {
  const children = node.children_kwd ?? [];
  const hasChildren = children.length > 0;
  const isSelected = selectedSkill === node.skill_kwd;
  const [expanded, setExpanded] = useState(depth < 1);
  const snippet = tooltipSnippet(node);
  const Icon = hasChildren ? (expanded ? FolderOpen : Folder) : FileText;

  return (
    <li>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className={cn(
              'group flex h-9 w-full items-center gap-2 rounded px-2 text-left text-sm text-text-secondary',
              'hover:bg-bg-card hover:text-text-primary',
              isSelected && 'bg-bg-card text-text-primary',
            )}
            style={{ paddingLeft: `${depth * 16 + 8}px` }}
            onClick={() => {
              onSelect(node.skill_kwd);
              if (hasChildren) setExpanded((value) => !value);
            }}
          >
            {hasChildren ? (
              <span className="inline-flex size-4 shrink-0 items-center justify-center">
                {expanded ? (
                  <ChevronDown className="size-3.5" />
                ) : (
                  <ChevronRight className="size-3.5" />
                )}
              </span>
            ) : (
              <span className="size-4 shrink-0" />
            )}
            <Icon className="size-4 shrink-0" />
            <span className="min-w-0 flex-1 truncate">
              {skillLabel(node.skill_kwd)}
            </span>
          </button>
        </TooltipTrigger>
        {snippet ? (
          <TooltipContent side="right" className="max-w-[28rem]">
            {snippet}
          </TooltipContent>
        ) : null}
      </Tooltip>
      {hasChildren && expanded ? (
        <ul className="mt-0.5">
          {children.map((child) => (
            <SkillTreeNode
              key={child.skill_kwd}
              node={child}
              depth={depth + 1}
              selectedSkill={selectedSkill}
              onSelect={onSelect}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

export default function DatasetSkillsPage() {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedSkill = searchParams.get('skill_kwd');
  const { data: tree, loading: treeLoading } = useFetchDatasetSkillTree();
  const roots = useMemo(
    () => tree?.skill_with_weight ?? [],
    [tree?.skill_with_weight],
  );

  const handleSelect = useCallback(
    (skillKwd: string) => {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        next.set('skill_kwd', skillKwd);
        return next;
      });
    },
    [setSearchParams],
  );

  useEffect(() => {
    if (!selectedSkill) {
      const first = firstSkill(roots);
      if (first) handleSelect(first);
    }
  }, [handleSelect, roots, selectedSkill]);

  const { data: page, loading: pageLoading } =
    useFetchDatasetSkillPage(selectedSkill);

  const markdownComponents = useMemo<Components>(
    () => ({
      p: ({ children, ...rest }) => (
        <p {...rest} className="my-4 leading-7 text-text-primary">
          {children}
        </p>
      ),
      a: ({ children, ...rest }) => (
        <a
          {...rest}
          className="text-accent-primary underline-offset-4 hover:underline"
        >
          {children}
        </a>
      ),
      table: ({ children, ...rest }) => (
        <div className="my-5 overflow-auto">
          <table {...rest} className="w-full border-collapse text-sm">
            {children}
          </table>
        </div>
      ),
      th: ({ children, ...rest }) => (
        <th
          {...rest}
          className="border border-border-button bg-bg-card px-3 py-2 text-left font-medium"
        >
          {children}
        </th>
      ),
      td: ({ children, ...rest }) => (
        <td {...rest} className="border border-border-button px-3 py-2">
          {children}
        </td>
      ),
    }),
    [],
  );

  const content = stripFrontmatter(page?.md_with_weight ?? '');

  return (
    <TooltipProvider>
      <div className="flex h-full min-h-0 flex-row bg-bg-base">
        <aside className="flex w-[22rem] shrink-0 flex-col border-r border-border-button">
          <header className="border-b border-border-button px-4 py-3">
            <h2 className="flex items-center gap-2 text-sm font-medium text-text-primary">
              <Sparkles className="size-4" />
              {t('datasetSkill.folders')}
            </h2>
          </header>
          <div className="flex-1 overflow-auto p-2">
            {treeLoading && roots.length === 0 ? (
              <div className="px-3 py-6 text-center text-sm text-text-secondary">
                {t('common.loading')}
              </div>
            ) : roots.length === 0 ? (
              <div className="px-3 py-6 text-center text-sm text-text-secondary">
                {t('datasetSkill.empty')}
              </div>
            ) : (
              <ul className="space-y-0.5">
                {roots.map((node) => (
                  <SkillTreeNode
                    key={node.skill_kwd}
                    node={node}
                    depth={0}
                    selectedSkill={selectedSkill}
                    onSelect={handleSelect}
                  />
                ))}
              </ul>
            )}
          </div>
        </aside>
        <section className="flex min-w-0 flex-1 flex-col overflow-auto">
          {!selectedSkill ? (
            <div className="flex flex-1 items-center justify-center text-text-secondary">
              {t('datasetSkill.selectFolder')}
            </div>
          ) : pageLoading && !page ? (
            <div className="flex flex-1 items-center justify-center text-text-secondary">
              {t('common.loading')}
            </div>
          ) : page ? (
            <>
              <header className="border-b border-border-button px-6 py-5">
                <div className="text-xs text-text-secondary">
                  {t('datasetSkill.currentFolder')}
                </div>
                <h1 className="mt-1 truncate text-2xl font-semibold text-text-primary">
                  {skillLabel(page.skill_kwd)}
                </h1>
              </header>
              <article className="prose max-w-none px-6 py-6 dark:prose-invert">
                <ReactMarkdown
                  remarkPlugins={[remarkGfm, remarkBreaks]}
                  components={markdownComponents}
                >
                  {content || t('datasetSkill.noContent')}
                </ReactMarkdown>
              </article>
            </>
          ) : (
            <div className="flex flex-1 items-center justify-center text-text-secondary">
              {t('datasetSkill.noContent')}
            </div>
          )}
        </section>
      </div>
    </TooltipProvider>
  );
}
