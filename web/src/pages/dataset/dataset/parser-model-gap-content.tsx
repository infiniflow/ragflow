import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';
import type { FileModelGap } from './utils';

/**
 * Builds the body of the missing-model modals (upload warning, parse-click
 * error). Strings are resolved with the caller's `t` because the static Modal
 * API renders content in a separate React root without the app providers.
 */
export function buildMissingModelModalContent(
  t: TFunction,
  gaps: FileModelGap[],
  hintKey: string,
): ReactNode {
  return (
    <div className="space-y-2">
      <ul className="list-disc pl-4 space-y-1">
        {gaps.map((gap, index) => (
          <li key={`${gap.name}-${index}`}>
            {t('knowledgeDetails.fileModelMissing', {
              name: gap.name,
              fileType: t(`flow.fileFormatOptions.${gap.fileType}`),
              model: t(
                gap.modelKind === 'asr'
                  ? 'knowledgeDetails.missingModelAsr'
                  : 'knowledgeDetails.missingModelVision',
              ),
            })}
          </li>
        ))}
      </ul>
      <p className="text-text-secondary">{t(hintKey)}</p>
    </div>
  );
}
