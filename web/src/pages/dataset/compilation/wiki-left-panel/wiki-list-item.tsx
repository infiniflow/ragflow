import { IArtifact, IArtifactTopic } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { FileText, Folder } from 'lucide-react';

type WikiTopicListItemProps = {
  topic: IArtifactTopic;
  onSelect: (topic: IArtifactTopic) => void;
};

export function WikiTopicListItem({ topic, onSelect }: WikiTopicListItemProps) {
  const handleClick = () => {
    onSelect(topic);
  };

  return (
    <li
      onClick={handleClick}
      className={cn(
        'flex items-center gap-2 px-3 py-2 rounded-md text-sm cursor-pointer',
        'text-text-secondary hover:bg-bg-base hover:text-text-primary',
      )}
    >
      <Folder className="size-4 shrink-0" />
      <span className="truncate">{topic.title}</span>
    </li>
  );
}

type WikiArtifactListItemProps = {
  item: IArtifact;
  isSelected: boolean;
  onSelect: (artifact: IArtifact) => void;
};

export function WikiArtifactListItem({
  item,
  isSelected,
  onSelect,
}: WikiArtifactListItemProps) {
  const handleClick = () => {
    onSelect(item);
  };

  return (
    <li
      onClick={handleClick}
      className={cn(
        'flex items-center justify-between gap-2 px-3 py-2 rounded-md text-sm cursor-pointer',
        'text-text-secondary hover:bg-bg-base hover:text-text-primary',
        isSelected && 'bg-bg-card text-text-primary',
      )}
    >
      <div className="flex items-center gap-2 min-w-0">
        <FileText className="size-4 shrink-0" />
        <span className="truncate">{item.title}</span>
      </div>
      {item.page_type && (
        <span className="text-text-disabled shrink-0 capitalize text-xs">
          {item.page_type}
        </span>
      )}
    </li>
  );
}
