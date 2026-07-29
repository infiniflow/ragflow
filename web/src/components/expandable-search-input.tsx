import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { Search, X } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface ExpandableSearchInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  inputClassName?: string;
  width?: number | string;
}

export function ExpandableSearchInput({
  value,
  onChange,
  placeholder,
  className,
  inputClassName,
  width = 192,
}: ExpandableSearchInputProps) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);

  const handleToggle = useCallback(() => {
    setIsOpen((prev) => {
      const next = !prev;
      if (!next) {
        onChange('');
      }
      return next;
    });
  }, [onChange]);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(e.target.value);
    },
    [onChange],
  );

  return (
    <div className={cn('relative flex items-center gap-2', className)}>
      <div
        className={cn(
          'transition-all duration-300 ease-in-out',
          isOpen ? 'opacity-100' : 'w-0 overflow-hidden opacity-0',
        )}
        style={{ width: isOpen ? width : 0 }}
      >
        <SearchInput
          value={value}
          onChange={handleChange}
          className={cn('w-full', inputClassName)}
          autoFocus={isOpen}
          placeholder={placeholder}
          suffix={
            isOpen ? (
              <button
                type="button"
                onClick={handleToggle}
                className="p-1 text-text-secondary hover:text-text-primary"
                aria-label={t('common.close', 'Close')}
              >
                <X className="h-4 w-4" />
              </button>
            ) : null
          }
        />
      </div>
      {!isOpen && (
        <Button
          variant="ghost"
          size="icon"
          type="button"
          onClick={handleToggle}
          aria-label={t('chunk.search', 'Search')}
        >
          <Search className="h-5 w-5" />
        </Button>
      )}
    </div>
  );
}
