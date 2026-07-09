import {
  SelectWithSearch,
  type SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import {
  type IStructureGraphTemplate,
  type StructureTemplateKind,
} from '@/interfaces/database/document-structure';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

interface RepresentationSelectProps {
  templates: IStructureGraphTemplate[];
  value?: string;
  onChange?: (value: string) => void;
}

function getKindLabel(
  t: (key: string) => string,
  kind: StructureTemplateKind,
): string {
  return t(`chunk.representationKinds.${kind}`);
}

export function RepresentationSelect({
  templates,
  value,
  onChange,
}: RepresentationSelectProps) {
  const { t } = useTranslation();

  const options = useMemo<SelectWithSearchFlagOptionType[]>(() => {
    return templates.map((template) => ({
      value: template.template_id,
      label: (
        <span className="flex items-center gap-2">
          <span className="truncate">{template.template_name}</span>
          <span className="text-xs text-text-secondary shrink-0">
            {getKindLabel(t, template.kind)}
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
      triggerClassName="w-1/2"
    />
  );
}
