import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useCompilationTemplateGroupOptions } from '@/hooks/use-compilation-template-group-request';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
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
  const { navigateToAgents } = useNavigatePage();
  const options = useCompilationTemplateGroupOptions();

  return (
    <RAGFlowFormItem
      name={name}
      label={t('knowledgeConfiguration.compilationTemplate')}
      labelLink={{
        text: t('knowledgeConfiguration.createTemplate'),
        onClick: navigateToAgents,
      }}
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
