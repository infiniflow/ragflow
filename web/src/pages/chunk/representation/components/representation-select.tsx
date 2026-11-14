import {
  SelectWithSearch,
  type SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { type IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import { formatKindLabel } from '@/utils/compilation-template-util';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

interface RepresentationSelectProps {
  templates: IStructureGraphTemplate[];
  value?: string;
  onChange?: (value: string) => void;
  triggerClassName?: string;
}

export function RepresentationSelect({
  templates,
  value,
  onChange,
  triggerClassName,
}: RepresentationSelectProps) {
  const { t } = useTranslation();
  const options = useMemo<SelectWithSearchFlagOptionType[]>(() => {
    return templates.map((template) => ({
      value: template.template_id,
      label: (
        <span className="flex items-center gap-2">
          <span className="truncate">{template.template_name}</span>
          <span className="text-xs text-text-secondary shrink-0">
            {formatKindLabel(t, template.kind)}
          </span>
        </span>
      ),
      keywords: [template.template_name, template.kind],
    }));
  }, [templates, t]);

  return (
    <SelectWithSearch
      options={options}
      value={value}
      onChange={onChange}
      // Compact header rows need w-fit: SelectWithSearch has no shrink-wrap
      // layer, so the trigger sizes to its label. Cap the width — template
      // names are server-returned text and truncate beyond the cap.
      triggerClassName={cn('w-fit max-w-60', triggerClassName)}
    />
  );
}
