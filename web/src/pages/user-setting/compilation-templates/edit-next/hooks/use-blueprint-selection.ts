import { CompilationTemplateKind } from '@/constants/compilation';
import {
  ICompilationTemplateBuiltin,
  IWikiPreset,
} from '@/interfaces/database/compilation-template';
import { useCallback, useMemo, useState } from 'react';
import { UseFormReturn } from 'react-hook-form';

import { FormSchemaType } from '../schema';
import { splitExampleToBlueprintFields } from '../utils';

type UseBlueprintSelectionParams = {
  form: UseFormReturn<FormSchemaType>;
  builtins: ICompilationTemplateBuiltin[];
  selectedTemplateIndex: number;
};

export function useBlueprintSelection({
  form,
  builtins,
  selectedTemplateIndex,
}: UseBlueprintSelectionParams) {
  const [selectedPresetId, setSelectedPresetId] = useState<string>();
  const [previewPreset, setPreviewPreset] = useState<IWikiPreset>();

  const instructionPath =
    `templates.${selectedTemplateIndex}.config.instruction` as const;
  const pageExamplePath =
    `templates.${selectedTemplateIndex}.config.page_example` as const;
  const useBlueprintPath =
    `templates.${selectedTemplateIndex}.config.use_blueprint` as const;

  const builtinExample = useMemo(() => {
    const builtin = builtins.find(
      (item) => item.kind === CompilationTemplateKind.Artifacts,
    );
    return String(builtin?.config?.example ?? '');
  }, [builtins]);

  const handleTogglePreset = useCallback(
    (preset: IWikiPreset) => {
      if (preset.id === selectedPresetId) {
        const { instruction, page_example } =
          splitExampleToBlueprintFields(builtinExample);
        form.setValue(instructionPath, instruction, {
          shouldValidate: false,
        });
        form.setValue(pageExamplePath, page_example, {
          shouldValidate: false,
        });
        form.setValue(useBlueprintPath, builtinExample.length > 0, {
          shouldValidate: false,
        });
        setSelectedPresetId(undefined);
        return;
      }

      form.setValue(instructionPath, preset.instruction, {
        shouldValidate: false,
      });
      form.setValue(pageExamplePath, preset.page_example, {
        shouldValidate: false,
      });
      form.setValue(useBlueprintPath, true, { shouldValidate: false });
      setSelectedPresetId(preset.id);
    },
    [
      builtinExample,
      form,
      instructionPath,
      pageExamplePath,
      selectedPresetId,
      useBlueprintPath,
    ],
  );

  const handlePreview = useCallback((preset: IWikiPreset) => {
    setPreviewPreset(preset);
  }, []);

  const handlePreviewOpenChange = useCallback((open: boolean) => {
    if (!open) {
      setPreviewPreset(undefined);
    }
  }, []);

  return {
    selectedPresetId,
    previewPreset,
    handleTogglePreset,
    handlePreview,
    handlePreviewOpenChange,
  };
}
