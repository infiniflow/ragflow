import { Button } from '@/components/ui/button';
import { Plus, Trash2 } from 'lucide-react';
import { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

interface FieldListBlockProps<T> {
  items: T[];
  onAdd: () => void;
  onRemove: (index: number) => void;
  renderItem: (item: T, index: number) => ReactNode;
  addLabel: string;
  emptyLabel?: string;
  minItems?: number;
}

/**
 * Generic add/delete repeater for a list of structured fields.
 * Used by Entity, Relation, Claim and Concept sections — each just
 * passes its own renderItem that draws the per-row inputs.
 */
export function FieldListBlock<T>({
  items,
  onAdd,
  onRemove,
  renderItem,
  addLabel,
  emptyLabel,
  minItems = 0,
}: FieldListBlockProps<T>) {
  const { t } = useTranslation();
  const canRemove = items.length > minItems;

  return (
    <div className="flex flex-col gap-3">
      {items.length === 0 && emptyLabel && (
        <p className="text-sm text-text-secondary">{emptyLabel}</p>
      )}
      {items.map((item, index) => (
        <div
          key={
            typeof item === 'object' && item !== null && 'id' in item
              ? String(item.id)
              : index
          }
          className="rounded-md border border-border-button p-3 flex flex-col gap-2 bg-bg-base"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs uppercase tracking-wide text-text-secondary">
              {t('knowledgeCompilation.field')} #{index + 1}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={!canRemove}
              onClick={() => onRemove(index)}
              aria-label={t('common.delete')}
            >
              <Trash2 className="size-3.5" />
            </Button>
          </div>
          {renderItem(item, index)}
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="self-start"
        onClick={onAdd}
      >
        <Plus className="size-3.5" />
        {addLabel}
      </Button>
    </div>
  );
}
