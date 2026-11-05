import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { buildValidModelIds, parseModelValue } from '@/utils/llm-util';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { useWatch } from 'react-hook-form';
import {
  LanguageFormField,
  ParserMethodFormField,
  RemoveHeaderFooterFormField,
  RmdirFormField,
  TcadpFormFields,
  TwoColumnCheckFormField,
} from './common-form-fields';
import { useOwnerTenantId } from '../../context';
import { CommonProps } from './interface';
import { DynamicPageRange } from './dynamic-page-range';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix } from './utils';

/** Renders PDF parsing controls shared by canvas and dataset configuration. */
export function PdfFormFields({ prefix }: CommonProps) {
  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);
  const parseMethod = useWatch({
    name: parseMethodName,
  });

  const ownerTenantId = useOwnerTenantId();
  const { data: addedModels } = useFetchAllAddedModels(
    undefined,
    ownerTenantId,
  );
  const modelIds = useMemo(
    () => buildValidModelIds(addedModels, ['image2text', 'ocr', 'vision']),
    [addedModels],
  );
  const method = parseMethod?.trim().toLowerCase();
  const isDefaultDeepDoc = !parseMethod || method === 'deepdoc';
  const ignoresCleanup = [
    ParseDocumentType.PlainText,
    ParseDocumentType.Docling,
    ParseDocumentType.OpenDataLoader,
    ParseDocumentType.TCADPParser,
    'monkeyocrv2',
    'plain_text',
    'plaintext',
    'mineru',
    'paddleocr',
    'somark',
  ].some((value) => value.toLowerCase() === method);
  // Keep registered analyzers usable even if model metadata is unavailable.
  // Hide tenant selections when the existing query can identify them.
  const cleanupShown =
    isDefaultDeepDoc ||
    (!ignoresCleanup &&
      !parseModelValue(parseMethod) &&
      !modelIds.has(parseMethod));

  const languageShown = useMemo(() => {
    return (
      !isEmpty(parseMethod) &&
      parseMethod !== ParseDocumentType.DeepDOC &&
      parseMethod !== ParseDocumentType.PlainText &&
      parseMethod !== ParseDocumentType.TCADPParser
    );
  }, [parseMethod]);

  useSetInitialLanguage({ prefix, languageShown });

  return (
    <>
      <TwoColumnCheckFormField prefix={prefix} />
      {cleanupShown && (
        <>
          <RmdirFormField prefix={prefix} />
          <RemoveHeaderFooterFormField prefix={prefix} />
        </>
      )}
      <ParserMethodFormField prefix={prefix} showMineruOptions />
      <DynamicPageRange prefix={prefix} />

      {languageShown && <LanguageFormField prefix={prefix}></LanguageFormField>}
      <TcadpFormFields prefix={prefix} />
    </>
  );
}
