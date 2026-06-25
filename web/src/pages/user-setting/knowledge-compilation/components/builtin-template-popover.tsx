import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { BuiltinCompilationTemplate } from '@/interfaces/database/compilation-template';
import { ChevronDown } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface BuiltinTemplatePopoverProps {
  onSelect: (template: BuiltinCompilationTemplate) => void;
}

/**
 * Top-right popup list of the five server-side built-in templates.
 * The list is cached for the session via React Query — the popover
 * doesn't refetch on every open.
 */
export function BuiltinTemplatePopover({
  onSelect,
}: BuiltinTemplatePopoverProps) {
  const { t } = useTranslation();
  const {
    data: builtins,
    loading,
    refetch,
  } = useFetchBuiltinCompilationTemplates();
  const [open, setOpen] = useState(false);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      setOpen(nextOpen);
      if (nextOpen) {
        refetch();
      }
    },
    [refetch],
  );

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm">
          {t('knowledgeCompilation.builtinTemplates')}
          <ChevronDown className="size-3.5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-1">
        {loading && (
          <p className="p-3 text-sm text-text-secondary">
            {t('common.loading')}
          </p>
        )}
        {!loading && builtins.length === 0 && (
          <p className="p-3 text-sm text-text-secondary">
            {t('knowledgeCompilation.noBuiltins')}
          </p>
        )}
        <ul className="flex flex-col">
          {builtins.map((b) => (
            <li key={b.id}>
              <button
                type="button"
                className="w-full text-left px-3 py-2 text-sm rounded hover:bg-bg-card focus:bg-bg-card focus:outline-none"
                onClick={() => {
                  setOpen(false);
                  onSelect(b);
                }}
              >
                <div className="font-medium">{b.display_name}</div>
              </button>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  );
}
