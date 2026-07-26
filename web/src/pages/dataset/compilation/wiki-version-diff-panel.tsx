import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { WikiDiffLine } from './interface';
import { getWikiDiffChanges, parseWikiDiff } from './utils/parse-wiki-diff';

type WikiVersionDiffPanelProps = {
  diff: string;
  title?: string;
};

type DiffLineProps = {
  line: WikiDiffLine;
};

function DiffLine({ line }: DiffLineProps) {
  return (
    <div
      className={cn(
        'p-2.5 rounded',
        line.type === 'added' && 'bg-[#3BA05C]/10',
        line.type === 'removed' && 'bg-[#D8494B]/10',
      )}
    >
      {line.content.length === 0 ? <br /> : line.content}
    </div>
  );
}

export function WikiVersionDiffPanel({
  diff,
  title,
}: WikiVersionDiffPanelProps) {
  const { t } = useTranslation();
  const changes = getWikiDiffChanges(parseWikiDiff(diff));

  return (
    <section className="flex flex-col h-full border-l border-border-button p-5">
      <header className="shrink-0 pb-5">
        <h3 className="font-semibold text-text-primary">
          {title || t('knowledgeDetails.versionDiff')}
        </h3>
      </header>
      <div className="flex-1 overflow-y-auto">
        {changes.length === 0 ? (
          <div className=" text-sm text-text-secondary">
            {t('knowledgeDetails.noDiffAvailable')}
          </div>
        ) : (
          <div className="space-y-5">
            {changes.map((line, index) => (
              <DiffLine key={index} line={line} />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
