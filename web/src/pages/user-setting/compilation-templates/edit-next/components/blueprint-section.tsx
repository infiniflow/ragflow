import { Collapse } from '@/components/collapse';
import MarkdownEditor from '@/components/markdown-editor';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Textarea } from '@/components/ui/textarea';
import { useFetchWikiPresets } from '@/hooks/use-compilation-template-request';
import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { useBlueprintSelection } from '../hooks/use-blueprint-selection';
import { FormSchemaType } from '../schema';

type BlueprintSectionProps = {
  form: UseFormReturn<FormSchemaType>;
  selectedTemplateIndex: number;
  builtins: ICompilationTemplateBuiltin[];
};

export function BlueprintSection({
  form,
  selectedTemplateIndex,
  builtins,
}: BlueprintSectionProps) {
  const { t } = useTranslation();
  const { data: presets } = useFetchWikiPresets();
  const { selectedValue, options, handleSelect, instructionPath, examplePath } =
    useBlueprintSelection({ form, selectedTemplateIndex, presets, builtins });

  if (presets.length === 0) {
    return null;
  }

  return (
    <section className="space-y-4 pt-4">
      <Collapse
        defaultOpen
        title={
          <h3 className="text-base font-medium">{t('setting.blueprints')}</h3>
        }
      >
        <div className="space-y-4">
          <SelectWithSearch
            value={selectedValue}
            onChange={handleSelect}
            options={options}
            placeholder={t('common.selectPlaceholder')}
          />

          <div className="space-y-4">
            <RAGFlowFormItem
              name={instructionPath}
              label={t('setting.instruction')}
            >
              <Textarea rows={8} resize={'vertical'} />
            </RAGFlowFormItem>

            <RAGFlowFormItem
              name={examplePath}
              label={t('setting.example')}
              className="flex h-[50vh] min-h-0 flex-col"
              valueClassName="flex-1 min-h-0"
            >
              {(field) => (
                <MarkdownEditor
                  content={String(field.value ?? '')}
                  onChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
          </div>
        </div>
      </Collapse>
    </section>
  );
}
