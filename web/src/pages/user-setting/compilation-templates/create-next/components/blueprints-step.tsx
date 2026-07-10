import MarkdownEditor from '@/components/markdown-editor';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { TreeDataItem, TreeView } from '@/components/ui/tree-view';
import { useFetchWikiPresets } from '@/hooks/use-compilation-template-request';
import { IWikiPreset } from '@/interfaces/database/compilation-template';
import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { groupBy } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { Checkbox } from '@/components/ui/checkbox';

type WikiPresetTreeItem = TreeDataItem & {
  _preset?: IWikiPreset;
};

type BlueprintsStepProps = {
  form: UseFormReturn<FormSchemaType>;
  selectedTemplateIndex: number;
  onBack: () => void;
  onSave: () => void;
  isLoading: boolean;
};

function BlueprintsEmptyState() {
  const { t } = useTranslation();

  return (
    <div className="flex-1 flex flex-col items-center p-8 text-center">
      <h3 className="text-2xl font-semibold text-text-primary mb-4">
        {t('setting.blueprints')}
      </h3>
      <p className="text-text-secondary max-w-xl mb-2">
        {t('setting.blueprintsPlaceholderDescription')}
      </p>
      <p className="text-text-secondary max-w-xl">
        {t('setting.blueprintsPlaceholderSkip')}
      </p>
    </div>
  );
}

const buildTreeFromPresets = (presets: IWikiPreset[]): WikiPresetTreeItem[] => {
  const grouped = groupBy(presets, (preset) => preset.topic);

  return Object.entries(grouped).map(([topic, items]) => ({
    id: topic,
    name: topic,
    children: items.map((preset, index) => ({
      id: preset.id,
      name: `${topic} ${index + 1}`,
      _preset: preset,
    })),
  }));
};

export function BlueprintsStep({
  form,
  selectedTemplateIndex,
  onBack,
  onSave,
  isLoading,
}: BlueprintsStepProps) {
  const { t } = useTranslation();
  const { data: presets = [] } = useFetchWikiPresets();
  const [selectedItemId, setSelectedItemId] = useState<string | undefined>();

  const treeData = useMemo(() => buildTreeFromPresets(presets), [presets]);

  const instructionPath =
    `templates.${selectedTemplateIndex}.config.instruction` as const;
  const pageExamplePath =
    `templates.${selectedTemplateIndex}.config.page_example` as const;
  const useBlueprintPath =
    `templates.${selectedTemplateIndex}.config.use_blueprint` as const;

  const pageExample = useWatch({
    control: form.control,
    name: pageExamplePath,
  });

  const instruction = useWatch({
    control: form.control,
    name: instructionPath,
  });

  const useBlueprint = useWatch({
    control: form.control,
    name: useBlueprintPath,
  });

  const selectedTemplateName = useMemo(() => {
    if (!selectedItemId) return '';
    const selectedItem = treeData
      .flatMap((node) => node.children ?? [])
      .find((child) => child.id === selectedItemId);
    return selectedItem?.name ?? '';
  }, [treeData, selectedItemId]);

  const hasTemplateData = Boolean(selectedItemId || instruction || pageExample);

  const handleSelect = useCallback(
    (item?: TreeDataItem) => {
      if (!item || item.children) return;
      const presetItem = item as WikiPresetTreeItem;
      const preset = presetItem._preset;
      if (!preset) return;

      setSelectedItemId(item.id);
      form.setValue(instructionPath, preset.instruction, {
        shouldValidate: false,
      });
      form.setValue(pageExamplePath, preset.page_example, {
        shouldValidate: false,
      });
    },
    [form, instructionPath, pageExamplePath],
  );

  const handleToggleBlueprint = useCallback(
    (checked: boolean | 'indeterminate') => {
      if (checked === 'indeterminate') return;
      form.setValue(useBlueprintPath, checked, { shouldValidate: false });
    },
    [form, useBlueprintPath],
  );

  const handlePageExampleChange = useCallback(
    (value: string) => {
      form.setValue(pageExamplePath, value, { shouldValidate: false });
    },
    [form, pageExamplePath],
  );

  return (
    <section className="flex-1 flex flex-col min-h-0">
      <div className="flex-1 min-h-0 flex">
        <div className="w-72 shrink-0 border-r border-border-button overflow-y-auto">
          <div className="text-text-secondary text-base px-5 pb-3 pt-10">
            {t('setting.blueprints')}
          </div>
          <TreeView
            data={treeData}
            initialSelectedItemId={selectedItemId}
            onSelectChange={handleSelect}
            expandAll
          />
        </div>

        <div className="flex-1 min-h-0 flex flex-col">
          {hasTemplateData && (
            <section className="flex justify-between p-5">
              <span>{selectedTemplateName}</span>
              <div className="space-x-2">
                <Checkbox
                  id="use-blueprint-checkbox"
                  checked={Boolean(useBlueprint)}
                  onCheckedChange={handleToggleBlueprint}
                />
                <span>{t('setting.useBlueprint')}</span>
              </div>
            </section>
          )}
          {hasTemplateData ? (
            <div className="flex-1 min-h-0 flex flex-col p-5 gap-4 overflow-y-auto">
              <RAGFlowFormItem
                name={instructionPath}
                label={t('setting.instruction')}
              >
                <Textarea rows={6} />
              </RAGFlowFormItem>

              <div className="flex-1 min-h-0 flex flex-col">
                <MarkdownEditor
                  content={String(pageExample ?? '')}
                  onChange={handlePageExampleChange}
                />
              </div>
            </div>
          ) : (
            <BlueprintsEmptyState />
          )}

          <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-between">
            <Button type="button" variant="outline" onClick={onBack}>
              {t('common.back')}
            </Button>
            <Button
              type="button"
              onClick={onSave}
              loading={isLoading}
              disabled={isLoading}
            >
              {t('common.save')}
            </Button>
          </footer>
        </div>
      </div>
    </section>
  );
}
