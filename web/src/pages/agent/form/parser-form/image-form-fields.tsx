import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Textarea } from '@/components/ui/textarea';
import { buildOptions } from '@/utils/form';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FileType, ImageParseMethod } from '../../constant/pipeline';
import { LanguageFormField, ParserMethodFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix, isForeignParseMethod } from './utils';

export function ImageFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const options = buildOptions(
    ImageParseMethod,
    t,
    'flow.imageParseMethodOptions',
  );
  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);

  const parseMethod = useWatch({
    name: parseMethodName,
  });

  const languageShown = useMemo(() => {
    return !isEmpty(parseMethod) && parseMethod !== ImageParseMethod.OCR;
  }, [parseMethod]);

  useEffect(() => {
    const current = form.getValues(parseMethodName);
    // On a file-type switch the field remounts and react-hook-form re-seeds it
    // from the node's saved form data, so it can hold another file type's
    // static parse method (e.g. DeepDOC) — reset it to OCR in that case too.
    if (isEmpty(current) || isForeignParseMethod(FileType.Image, current)) {
      form.setValue(parseMethodName, ImageParseMethod.OCR, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, parseMethodName]);

  useSetInitialLanguage({ prefix, languageShown });

  return (
    <>
      <ParserMethodFormField
        prefix={prefix}
        optionsWithoutLLM={options}
      ></ParserMethodFormField>
      {languageShown && <LanguageFormField prefix={prefix}></LanguageFormField>}
      {languageShown && (
        <RAGFlowFormItem
          name={buildFieldNameWithPrefix('system_prompt', prefix)}
          label={t('flow.systemPrompt')}
        >
          <Textarea placeholder={t('flow.systemPromptPlaceholder')} />
        </RAGFlowFormItem>
      )}
    </>
  );
}
