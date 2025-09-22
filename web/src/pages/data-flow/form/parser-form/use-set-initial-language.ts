import { crossLanguageOptions } from '@/components/cross-language-form-field';
import { isEmpty } from 'lodash';
import { useEffect } from 'react';
import { useFormContext } from 'react-hook-form';
import { buildFieldNameWithPrefix } from './utils';

export function useSetInitialLanguage({
  prefix,
  languageShown,
}: {
  prefix: string;
  languageShown: boolean;
}) {
  const form = useFormContext();
  const lang = form.getValues(buildFieldNameWithPrefix('lang', prefix));

  useEffect(() => {
    if (languageShown && isEmpty(lang)) {
      form.setValue(
        buildFieldNameWithPrefix('lang', prefix),
        crossLanguageOptions[0].value,
        {
          shouldValidate: true,
          shouldDirty: true,
        },
      );
    }
  }, [form, lang, languageShown, prefix]);
}
