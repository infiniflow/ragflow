import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useFetchAllCompilationTemplateGroups } from '@/hooks/use-compilation-template-group-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { SelectWithSearch } from './originui/select-with-search';

type CompilationTemplateFormFieldProps = {
  horizontal?: boolean;
  name?: string;
};

export function CompilationTemplateFormField({
  horizontal,
  name = 'parser_config.compilation_template_group_id',
}: CompilationTemplateFormFieldProps) {
  const { t } = useTranslation();
  const { groups } = useFetchAllCompilationTemplateGroups();

  const options = useMemo(
    () => groups?.map((group) => ({ label: group.name, value: group.id })),
    [groups],
  );

  return (
    <RAGFlowFormItem
      name={name}
      label={t('knowledgeConfiguration.compilationTemplate')}
      className="pb-4"
      horizontal={horizontal}
    >
      {(field) => (
        <SelectWithSearch
          value={field.value}
          onChange={field.onChange}
          options={options}
        />
      )}
    </RAGFlowFormItem>
  );
}
