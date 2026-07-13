import { CompilationTemplateKind } from '@/constants/compilation';
import { FormSchemaType } from '@/pages/user-setting/compilation-templates/create-next/schema';
import { UseFormReturn, useWatch } from 'react-hook-form';

export const useTemplateAddButton = (
  form: UseFormReturn<FormSchemaType>,
  kindOptions: { label: string; value: string }[],
) => {
  const templates =
    useWatch({ control: form.control, name: 'templates' }) ?? [];

  const hasTemplateWithoutKind = templates.some((template) => !template.kind);
  const hasArtifactsTemplate = templates.some(
    (template) => template.kind === CompilationTemplateKind.Artifacts,
  );
  const selectedKinds = new Set(
    templates
      .map((template) => template.kind)
      .filter((kind): kind is string => Boolean(kind)),
  );
  const allKindsSelected =
    kindOptions.length > 0 &&
    kindOptions.every((option) => selectedKinds.has(option.value));

  const isAddButtonHidden =
    hasTemplateWithoutKind || hasArtifactsTemplate || allKindsSelected;

  return { templates, isAddButtonHidden };
};
