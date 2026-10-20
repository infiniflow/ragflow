import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';
import { ParserGapReason, ParserModelKind } from './constant';
import { pickByGapKind, type FileParserGap } from './utils';

/**
 * Builds the body of the parser-gap modals (upload warning, parse-click
 * error). Strings are resolved with the caller's `t` because the static Modal
 * API renders content in a separate React root without the app providers.
 * The hint follows the gap kind: missing models point to adding the model,
 * unsupported types to reselecting the parse method.
 */
export function buildParserGapModalContent(
  t: TFunction,
  gaps: FileParserGap[],
  hintKeys: { missingModel: string; unsupportedType: string },
): ReactNode {
  const hintKey = pickByGapKind(gaps, hintKeys);
  return (
    <div className="space-y-2">
      <ul className="list-disc pl-4 space-y-1">
        {gaps.map((gap, index) => (
          <li key={`${gap.name}-${index}`}>
            {gap.reason === ParserGapReason.MissingModel
              ? t('knowledgeDetails.fileModelMissing', {
                  name: gap.name,
                  fileType: t(`flow.fileFormatOptions.${gap.fileType}`),
                  model: t(
                    gap.modelKind === ParserModelKind.Asr
                      ? 'knowledgeDetails.missingModelAsr'
                      : 'knowledgeDetails.missingModelVision',
                  ),
                })
              : t('knowledgeDetails.fileTypeUnsupported', {
                  name: gap.name,
                  fileType: t(`flow.fileFormatOptions.${gap.fileType}`),
                })}
          </li>
        ))}
      </ul>
      <p className="text-text-secondary">{t(hintKey)}</p>
    </div>
  );
}
