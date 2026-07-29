import { SelectWithSearchFlagOptionType } from '@/components/originui/select-with-search';
import { IWikiPreset } from '@/interfaces/database/compilation-template';
import { useCallback, useMemo, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '../schema';

export const CustomBlueprintValue = '__custom__';

type UseBlueprintSelectionParams = {
  form: UseFormReturn<FormSchemaType>;
  selectedTemplateIndex: number;
  presets: IWikiPreset[];
};

const isSameBlueprintContent = (
  preset: IWikiPreset,
  instruction: string,
  pageExample: string,
) =>
  preset.instruction.trim() === instruction.trim() &&
  preset.page_example.trim() === pageExample.trim();

export function useBlueprintSelection({
  form,
  selectedTemplateIndex,
  presets,
}: UseBlueprintSelectionParams) {
  const { t } = useTranslation();
  const [explicitValue, setExplicitValue] = useState<string>();

  const instructionPath =
    `templates.${selectedTemplateIndex}.config.instruction` as const;
  const pageExamplePath =
    `templates.${selectedTemplateIndex}.config.page_example` as const;
  const useBlueprintPath =
    `templates.${selectedTemplateIndex}.config.use_blueprint` as const;

  const instruction = useWatch({
    control: form.control,
    name: instructionPath,
  });
  const pageExample = useWatch({
    control: form.control,
    name: pageExamplePath,
  });

  const matchedPresetId = useMemo(() => {
    const currentInstruction = String(instruction ?? '');
    const currentPageExample = String(pageExample ?? '');
    if (!currentInstruction.trim() && !currentPageExample.trim()) {
      return undefined;
    }
    return presets.find((preset) =>
      isSameBlueprintContent(preset, currentInstruction, currentPageExample),
    )?.id;
  }, [instruction, pageExample, presets]);

  const selectedValue =
    explicitValue ?? matchedPresetId ?? CustomBlueprintValue;

  const options = useMemo<SelectWithSearchFlagOptionType[]>(
    () => [
      ...presets.map((preset) => ({
        label: preset.id,
        value: preset.id,
      })),
      { label: t('setting.custom'), value: CustomBlueprintValue },
    ],
    [presets, t],
  );

  const handleSelect = useCallback(
    (value: string) => {
      if (value === selectedValue) return;
      setExplicitValue(value);

      if (value === CustomBlueprintValue) {
        const defaultConfig =
          form.formState.defaultValues?.templates?.[selectedTemplateIndex]
            ?.config;
        form.setValue(
          instructionPath,
          String(defaultConfig?.instruction ?? ''),
          { shouldValidate: false },
        );
        form.setValue(
          pageExamplePath,
          String(defaultConfig?.page_example ?? ''),
          { shouldValidate: false },
        );
      } else {
        const preset = presets.find((item) => item.id === value);
        if (!preset) return;
        form.setValue(instructionPath, preset.instruction, {
          shouldValidate: false,
        });
        form.setValue(pageExamplePath, preset.page_example, {
          shouldValidate: false,
        });
      }

      form.setValue(useBlueprintPath, true, { shouldValidate: false });
    },
    [
      form,
      instructionPath,
      pageExamplePath,
      presets,
      selectedTemplateIndex,
      selectedValue,
      useBlueprintPath,
    ],
  );

  const handlePageExampleChange = useCallback(
    (value: string) => {
      form.setValue(pageExamplePath, value, { shouldValidate: false });
    },
    [form, pageExamplePath],
  );

  return {
    selectedValue,
    options,
    handleSelect,
    instructionPath,
    pageExample,
    handlePageExampleChange,
  };
}
