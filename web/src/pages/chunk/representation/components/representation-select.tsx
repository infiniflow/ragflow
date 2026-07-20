import {
  SelectWithSearch,
  type SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { type IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { formatKindLabel } from '@/utils/compilation-template-util';
import { useMemo } from 'react';

interface RepresentationSelectProps {
  templates: IStructureGraphTemplate[];
  value?: string;
  onChange?: (value: string) => void;
}

export function RepresentationSelect({
  templates,
  value,
  onChange,
}: RepresentationSelectProps) {
  const options = useMemo<SelectWithSearchFlagOptionType[]>(() => {
    return templates.map((template) => ({
      value: template.template_id,
      label: (
        <span className="flex items-center gap-2">
          <span className="truncate">{template.template_name}</span>
          <span className="text-xs text-text-secondary shrink-0">
            {formatKindLabel(template.kind)}
          </span>
        </span>
      ),
      keywords: [template.template_name, template.kind],
    }));
  }, [templates]);

  return (
    <SelectWithSearch
      options={options}
      value={value}
      onChange={onChange}
      triggerClassName="w-1/2"
    />
  );
}
