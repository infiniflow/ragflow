import { CompilationTemplateKind } from '@/constants/compilation';
import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';

export const useAvailableKindOptions = (
  form: UseFormReturn<FormSchemaType>,
  kindOptions: { label: string; value: string }[],
  selectedTemplateIndex: number,
) => {
  const kind = useWatch({
    control: form.control,
    name: `templates.${selectedTemplateIndex}.kind`,
  });

  const templates = useWatch({ control: form.control, name: 'templates' });

  const availableKindOptions = useMemo(() => {
    const otherSelectedKinds = new Set(
      templates
        ?.filter((_, index) => index !== selectedTemplateIndex)
        .map((template) => template.kind)
        .filter((value): value is string => Boolean(value)) ?? [],
    );

    const hasOtherNonArtifactsKind = Array.from(otherSelectedKinds).some(
      (value) => value !== CompilationTemplateKind.Artifacts,
    );
    const hasOtherArtifactsKind = otherSelectedKinds.has(
      CompilationTemplateKind.Artifacts,
    );

    return kindOptions.filter((option) => {
      if (option.value === kind) return true;
      if (otherSelectedKinds.has(option.value)) return false;
      if (
        hasOtherNonArtifactsKind &&
        option.value === CompilationTemplateKind.Artifacts
      ) {
        return false;
      }
      if (
        hasOtherArtifactsKind &&
        option.value !== CompilationTemplateKind.Artifacts
      ) {
        return false;
      }
      return true;
    });
  }, [kindOptions, kind, selectedTemplateIndex, templates]);

  return availableKindOptions;
};
