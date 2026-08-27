import { Collapse } from '@/components/collapse';
import { ModelTreeSelectFormField } from '@/components/model-tree-select';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import {
  ICompilationTemplateBuiltin,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { Trash2 } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { CompilationTemplateKind } from '@/constants/compilation';
import { useTemplateKindChange } from '../hooks/use-template-kind-change';
import { FormSchemaType } from '../schema';
import {
  DefaultFieldKeys,
  getFieldKeyOrder,
  isConfigMetaKey,
  SectionTitleKeyMap,
  sortSectionNames,
} from '../utils';
import { FieldsSection } from './fields-section';
import { TreeTemplateFields } from './tree-template-fields';

type TemplateCardProps = {
  index: number;
  form: UseFormReturn<FormSchemaType>;
  kindOptions: { label: string; value: string }[];
  builtins: ICompilationTemplateBuiltin[];
  onRemove: (index: number) => void;
  canRemove: boolean;
};

export function TemplateCard({
  index,
  form,
  kindOptions,
  builtins,
  onRemove,
  canRemove,
}: TemplateCardProps) {
  const { t } = useTranslation();

  const kind = useWatch({
    control: form.control,
    name: `templates.${index}.kind`,
  });

  const builtinTemplate = useMemo(
    () => builtins.find((template) => template.kind === kind),
    [builtins, kind],
  );

  const sectionNames = useMemo(() => {
    const names = Object.keys(builtinTemplate?.config ?? {}).filter(
      (key) => !isConfigMetaKey(key),
    );
    return sortSectionNames(names);
  }, [builtinTemplate]);

  const handleRemove = useCallback(() => {
    onRemove(index);
  }, [index, onRemove]);

  const handleRemoveClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      e.stopPropagation();
      handleRemove();
    },
    [handleRemove],
  );

  const handleKindChange = useTemplateKindChange({
    form,
    index,
    builtins,
  });

  return (
    <Card className="border-border-button">
      <CardContent className="p-4">
        <Collapse
          title={
            <div>
              <span className="text-sm font-medium text-text-primary">
                {t('setting.template')} #{index + 1}
              </span>
              <span className="ml-2 text-xs text-text-secondary">
                {kindOptions.find((option) => option.value === kind)?.label}
              </span>
            </div>
          }
          rightContent={
            canRemove && (
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                onClick={handleRemoveClick}
                className="text-text-secondary hover:text-state-error"
              >
                <Trash2 className="size-4" />
              </Button>
            )
          }
          defaultOpen
        >
          <div className="space-y-4">
            {kind !== CompilationTemplateKind.Tree && (
              <RAGFlowFormItem
                name={`templates.${index}.name`}
                label={t('setting.templateName')}
              >
                <Input placeholder={t('common.namePlaceholder')} />
              </RAGFlowFormItem>
            )}

            <RAGFlowFormItem
              name={`templates.${index}.description`}
              label={t('setting.templateDescription')}
            >
              <Textarea
                placeholder={t('common.descriptionPlaceholder')}
                rows={2}
              />
            </RAGFlowFormItem>

            <ModelTreeSelectFormField
              name={`templates.${index}.llm_id`}
              label={t('setting.llmForExtraction')}
              required
            />

            <RAGFlowFormItem
              name={`templates.${index}.kind`}
              label={t('knowledgeCompilation.builtinTemplates')}
              required
            >
              {(field) => (
                <SelectWithSearch
                  value={field.value}
                  onChange={(value) => handleKindChange(field, value)}
                  disabled={field.disabled}
                  options={kindOptions}
                  placeholder={t('common.selectPlaceholder')}
                />
              )}
            </RAGFlowFormItem>

            {kind === CompilationTemplateKind.Artifacts && (
              <RAGFlowFormItem
                name={`templates.${index}.config.example`}
                label={t('setting.example')}
              >
                <Textarea
                  placeholder={t('setting.examplePlaceholder')}
                  rows={4}
                />
              </RAGFlowFormItem>
            )}

            {kind === CompilationTemplateKind.Tree ? (
              <TreeTemplateFields index={index} />
            ) : (
              sectionNames.map((sectionName, sectionIndex) => {
                const builtinSection = builtinTemplate?.config?.[sectionName];

                const firstField = (
                  builtinSection as ICompilationTemplateSection | undefined
                )?.fields?.[0];
                const firstFieldKeys = firstField
                  ? Object.keys(firstField)
                  : DefaultFieldKeys;
                const fieldKeys = getFieldKeyOrder(firstFieldKeys);

                return (
                  <FieldsSection
                    key={sectionName}
                    name={
                      `templates.${index}.config.${sectionName}` as `templates.${number}.config.${string}`
                    }
                    title={t(SectionTitleKeyMap[sectionName] ?? sectionName)}
                    fieldKeys={fieldKeys}
                    defaultOpen={sectionIndex === 0}
                    builtinSection={
                      builtinSection as ICompilationTemplateSection | undefined
                    }
                  />
                );
              })
            )}

            <RAGFlowFormItem
              name={`templates.${index}.config.global_rules`}
              label={t('setting.globalRules')}
            >
              <Textarea
                placeholder={t('setting.globalRulesPlaceholder')}
                rows={4}
              />
            </RAGFlowFormItem>
          </div>
        </Collapse>
      </CardContent>
    </Card>
  );
}
