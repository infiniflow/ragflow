import { useMemo } from 'react';
import { ArrayPath, UseFormReturn } from 'react-hook-form';

import {
  ICompilationTemplateBuiltin,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { FormSchemaType } from '@/pages/user-setting/compilation-templates/edit-template/schema';

export const useTemplateSectionData = (
  form: UseFormReturn<FormSchemaType>,
  selectedTemplateIndex: number,
  activeSectionTab: string,
  builtinTemplate: ICompilationTemplateBuiltin | undefined,
  editingFieldIndex: number | null,
) => {
  const activeSectionPath = `templates.${selectedTemplateIndex}.config.${activeSectionTab}`;
  const activeFieldsPath =
    `${activeSectionPath}.fields` as ArrayPath<FormSchemaType>;

  const builtinSection = useMemo(() => {
    return builtinTemplate?.config?.[activeSectionTab] as
      | ICompilationTemplateSection
      | undefined;
  }, [activeSectionTab, builtinTemplate?.config]);

  const editingField = useMemo(() => {
    if (editingFieldIndex === null) return undefined;
    return ((form.getValues(activeFieldsPath) as
      | Record<string, string>[]
      | undefined) ?? [])[editingFieldIndex];
  }, [activeFieldsPath, editingFieldIndex, form]);

  return {
    activeSectionPath,
    activeFieldsPath,
    builtinSection,
    editingField,
  };
};
