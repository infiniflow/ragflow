import { SelectWithSearchFlagOptionType } from '@/components/originui/select-with-search';
import {
  ICompilationTemplateBuiltin,
  IWikiPreset,
} from '@/interfaces/database/compilation-template';
import { capitalize, lowerCase } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '../schema';

export const CustomBlueprintValue = '__custom__';

type UseBlueprintSelectionParams = {
  form: UseFormReturn<FormSchemaType>;
  selectedTemplateIndex: number;
  presets: IWikiPreset[];
  builtins: ICompilationTemplateBuiltin[];
};

const isSameBlueprintContent = (
  preset: IWikiPreset,
  instruction: string,
  example: string,
) =>
  preset.instruction.trim() === instruction.trim() &&
  preset.example.trim() === example.trim();

export function useBlueprintSelection({
  form,
  selectedTemplateIndex,
  presets,
  builtins,
}: UseBlueprintSelectionParams) {
  const { t } = useTranslation();
  const [explicitValue, setExplicitValue] = useState<string>();

  const kindPath = `templates.${selectedTemplateIndex}.kind` as const;
  const instructionPath =
    `templates.${selectedTemplateIndex}.config.instruction` as const;
  const examplePath =
    `templates.${selectedTemplateIndex}.config.example` as const;
  const useBlueprintPath =
    `templates.${selectedTemplateIndex}.config.use_blueprint` as const;

  const kind = useWatch({
    control: form.control,
    name: kindPath,
  });
  const instruction = useWatch({
    control: form.control,
    name: instructionPath,
  });
  const example = useWatch({
    control: form.control,
    name: examplePath,
  });

  const matchedPresetId = useMemo(() => {
    const currentInstruction = String(instruction ?? '');
    const currentExample = String(example ?? '');
    if (!currentInstruction.trim() && !currentExample.trim()) {
      return undefined;
    }
    return presets.find((preset) =>
      isSameBlueprintContent(preset, currentInstruction, currentExample),
    )?.id;
  }, [instruction, example, presets]);

  const selectedValue =
    explicitValue ?? matchedPresetId ?? CustomBlueprintValue;

  const options = useMemo<SelectWithSearchFlagOptionType[]>(
    () => [
      ...presets.map((preset) => ({
        label: capitalize(lowerCase(preset.id)),
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
        const builtinTemplate = builtins.find(
          (template) => template.kind === kind,
        );
        const defaultInstruction =
          typeof builtinTemplate?.config?.instruction === 'string'
            ? builtinTemplate.config.instruction
            : '';
        const defaultPageExample =
          typeof builtinTemplate?.config?.example === 'string'
            ? builtinTemplate.config.example
            : '';
        form.setValue(instructionPath, defaultInstruction, {
          shouldValidate: false,
        });
        form.setValue(examplePath, defaultPageExample, {
          shouldValidate: false,
        });
      } else {
        const preset = presets.find((item) => item.id === value);
        if (!preset) return;
        form.setValue(instructionPath, preset.instruction, {
          shouldValidate: false,
        });
        form.setValue(examplePath, preset.example, {
          shouldValidate: false,
        });
      }

      form.setValue(useBlueprintPath, true, { shouldValidate: false });
    },
    [
      builtins,
      form,
      instructionPath,
      kind,
      examplePath,
      presets,
      selectedValue,
      useBlueprintPath,
    ],
  );

  return {
    selectedValue,
    options,
    handleSelect,
    instructionPath,
    examplePath,
  };
}
