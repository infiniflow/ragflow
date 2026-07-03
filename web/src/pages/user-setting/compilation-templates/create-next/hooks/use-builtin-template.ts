import { useMemo } from 'react';

import {
  ICompilationTemplateBuiltin,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { isConfigMetaKey } from '@/pages/user-setting/compilation-templates/edit-template/utils';

export const useBuiltinTemplate = (
  builtins: ICompilationTemplateBuiltin[],
  kind: string,
) => {
  const builtinTemplate = useMemo(
    () => builtins.find((template) => template.kind === kind),
    [builtins, kind],
  );

  const sectionNames = useMemo(() => {
    return Object.keys(builtinTemplate?.config ?? {}).filter((key) => {
      if (isConfigMetaKey(key)) return false;
      const section = builtinTemplate?.config?.[key];
      return (
        section &&
        typeof section === 'object' &&
        Array.isArray((section as ICompilationTemplateSection).fields)
      );
    });
  }, [builtinTemplate]);

  return { builtinTemplate, sectionNames };
};
