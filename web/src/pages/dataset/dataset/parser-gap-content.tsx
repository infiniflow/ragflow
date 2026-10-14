import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';
import { ParserGapReason, ParserModelKind } from './constant';
import {
  BuiltinSwitchTargetLabels,
  pickByGapKind,
  pickBySwitchFate,
  type FileParserGap,
  type UploadFileParserGap,
} from './utils';

/**
 * Builds the body of the parser-gap modals (upload warning, parse-click
 * error). Strings are resolved with the caller's `t` because the static Modal
 * API renders content in a separate React root without the app providers.
 *
 * Two entry points share one layout: `buildParserGapModalContent` renders
 * already-uploaded documents (parse-click validation), while
 * `buildUploadGapModalContent` renders pre-upload files and annotates each
 * unsupported file with its switch fate — the built-in pipeline the backend
 * will switch it to, or the need to reselect a parse method manually.
 */

type GapHintKeys = { missingModel: string; unsupportedType: string };

type UploadGapHintKeys = {
  missingModel: string;
  autoSwitch: string;
  manualSwitch: string;
  mixedSwitch: string;
};

function renderGapLine(t: TFunction, gap: FileParserGap): ReactNode {
  if (gap.reason === ParserGapReason.MissingModel) {
    return t('knowledgeDetails.fileModelMissing', {
      name: gap.name,
      fileType: t(`flow.fileFormatOptions.${gap.fileType}`),
      model: t(
        gap.modelKind === ParserModelKind.Asr
          ? 'knowledgeDetails.missingModelAsr'
          : 'knowledgeDetails.missingModelVision',
      ),
    });
  }
  return t('knowledgeDetails.fileTypeUnsupported', {
    name: gap.name,
    fileType: t(`flow.fileFormatOptions.${gap.fileType}`),
  });
}

function renderUploadGapLine(
  t: TFunction,
  gap: UploadFileParserGap,
): ReactNode {
  if (gap.reason === ParserGapReason.MissingModel) {
    return renderGapLine(t, gap);
  }
  const fileType = t(`flow.fileFormatOptions.${gap.fileType}`);
  return gap.autoSwitchTarget
    ? t('knowledgeDetails.fileTypeUnsupportedAutoSwitch', {
        name: gap.name,
        fileType,
        parser: BuiltinSwitchTargetLabels[gap.autoSwitchTarget],
      })
    : t('knowledgeDetails.fileTypeUnsupportedManualSwitch', {
        name: gap.name,
        fileType,
      });
}

function buildContent<G extends FileParserGap>(
  t: TFunction,
  gaps: G[],
  hintKey: string,
  renderLine: (t: TFunction, gap: G) => ReactNode,
): ReactNode {
  return (
    <div className="space-y-2">
      <ul className="list-disc pl-4 space-y-1">
        {gaps.map((gap, index) => (
          <li key={`${gap.name}-${index}`}>{renderLine(t, gap)}</li>
        ))}
      </ul>
      <p className="text-text-secondary">{t(hintKey)}</p>
    </div>
  );
}

export function buildParserGapModalContent(
  t: TFunction,
  gaps: FileParserGap[],
  hintKeys: GapHintKeys,
): ReactNode {
  return buildContent(t, gaps, pickByGapKind(gaps, hintKeys), renderGapLine);
}

export function buildUploadGapModalContent(
  t: TFunction,
  gaps: UploadFileParserGap[],
  hintKeys: UploadGapHintKeys,
): ReactNode {
  return buildContent(
    t,
    gaps,
    pickBySwitchFate(gaps, hintKeys),
    renderUploadGapLine,
  );
}
