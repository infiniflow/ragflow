import { useMemo } from 'react';

import {
  ICompilationTemplateBuiltin,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import {
  isConfigMetaKey,
  sortSectionNames,
} from '@/pages/user-setting/compilation-templates/create-next/utils';

export const useBuiltinTemplate = (
  builtins: ICompilationTemplateBuiltin[],
  kind: string,
) => {
  const builtinTemplate = useMemo(
    () => builtins.find((template) => template.kind === kind),
    [builtins, kind],
  );

  const sectionNames = useMemo(() => {
    const names = Object.keys(builtinTemplate?.config ?? {}).filter((key) => {
      if (isConfigMetaKey(key)) return false;
      const section = builtinTemplate?.config?.[key];
      return (
        section &&
        typeof section === 'object' &&
        Array.isArray((section as ICompilationTemplateSection).fields)
      );
    });
    return sortSectionNames(names);
  }, [builtinTemplate]);

  return { builtinTemplate, sectionNames };
};
