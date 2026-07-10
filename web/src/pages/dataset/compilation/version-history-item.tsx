import { IWikiCommit } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';

type VersionHistoryItemProps = {
  version: IWikiCommit;
  isSelected: boolean;
  onSelect: (version: IWikiCommit) => void;
};

export function VersionHistoryItem({
  version,
  isSelected,
  onSelect,
}: VersionHistoryItemProps) {
  const handleClick = () => {
    onSelect(version);
  };

  return (
    <button
      onClick={handleClick}
      className={cn(
        'w-full text-left px-6 py-4 border-b border-border-button transition-colors rounded',
        isSelected ? 'bg-bg-card' : 'hover:bg-bg-card/50',
      )}
    >
      <div className="flex items-center gap-2 mb-1">
        <span className="text-text-primary truncate">{version.title}</span>
        <span
          className={cn(
            'size-2 rounded-full shrink-0',
            isSelected ? 'bg-accent-primary' : 'bg-transparent',
          )}
        />
      </div>
      <div className="space-y-1">
        {version.comments && (
          <p className="text-text-secondary">{version.comments}</p>
        )}
        <div className="flex items-center gap-2 text-xs text-text-disabled">
          <span>{version.user_nickname}</span>
          <span>·</span>
          <time dateTime={formatDate(version.create_time)}>
            {formatDate(version.create_time)}
          </time>
        </div>
      </div>
    </button>
  );
}
