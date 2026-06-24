import { RAGFlowFormItem } from '@/components/ragflow-form';
import { MultiSelect } from '@/components/ui/multi-select';
import { useFetchAllCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { useTranslation } from 'react-i18next';

type CompilationTemplateFormFieldProps = {
  horizontal?: boolean;
};

export function CompilationTemplateFormField({
  horizontal,
}: CompilationTemplateFormFieldProps) {
  const { t } = useTranslation();
  const { templates } = useFetchAllCompilationTemplates();

  const options = (templates ?? []).map((template) => ({
    label: (
      <div className="flex items-center">
        {template.name}
        <span className="text-text-secondary pl-3 text-xs">
          {t(`knowledgeCompilation.kind.${template.kind}`)}
        </span>
      </div>
    ),
    value: template.id,
  }));

  return (
    <RAGFlowFormItem
      name="parser_config.compilation_template_ids"
      label={t('knowledgeConfiguration.compilationTemplate')}
      className="pb-4"
      horizontal={horizontal}
    >
      {(field) => (
        <MultiSelect
          options={options}
          onValueChange={field.onChange}
          defaultValue={field.value}
          maxCount={100}
          modalPopover
        />
      )}
    </RAGFlowFormItem>
  );
}
