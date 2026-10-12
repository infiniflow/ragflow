import { Operator } from '@/constants/agent';
import { FileType, FileTypeSuffixMap } from '@/constants/file';
import type { IDocumentInfo } from '@/interfaces/database/document';
import { getExtension } from '@/utils/document-util';
import {
  getOperatorType,
  transformParserConfigSetups,
} from '@/utils/pipeline-operator';
import {
  IngestionTaskStatus,
  ParserGapReason,
  ParserModelKind,
  RunningStatus,
} from './constant';

/** Ingestion statuses that represent an active or canceling parse task on Go. */
const activeIngestionStatuses = new Set<IngestionTaskStatus>([
  IngestionTaskStatus.CREATED,
  IngestionTaskStatus.SCHEDULED,
  IngestionTaskStatus.RUNNING,
  IngestionTaskStatus.STOPPING,
]);

/**
 * Maps a Go `ingestion_status` value onto the legacy `RunningStatus`
 * vocabulary consumed by the document list UI (icons, dots, labels).
 *
 * - CREATED/SCHEDULED -> QUEUED (task enqueued, worker has not started)
 * - RUNNING/STOPPING  -> RUNNING (parse in flight; STOPPING keeps the
 *   in-progress presentation with the cancel button disabled)
 * - COMPLETED         -> DONE
 * - FAILED            -> FAIL
 * - STOPPED           -> CANCEL
 * - UNSTART/undefined -> UNSTART
 *
 * @param status - Raw `ingestion_status` from a Go document response.
 * @returns The display/action status used by existing UI components.
 *
 * @example
 * ingestionStatusToRunningStatus(IngestionTaskStatus.COMPLETED);
 * // => RunningStatus.DONE
 */
export const ingestionStatusToRunningStatus = (
  status?: IngestionTaskStatus,
): RunningStatus => {
  switch (status) {
    case IngestionTaskStatus.CREATED:
    case IngestionTaskStatus.SCHEDULED:
      return RunningStatus.QUEUED;
    case IngestionTaskStatus.RUNNING:
    case IngestionTaskStatus.STOPPING:
      return RunningStatus.RUNNING;
    case IngestionTaskStatus.COMPLETED:
      return RunningStatus.DONE;
    case IngestionTaskStatus.FAILED:
      return RunningStatus.FAIL;
    case IngestionTaskStatus.STOPPED:
      return RunningStatus.CANCEL;
    case IngestionTaskStatus.UNSTART:
    default:
      return RunningStatus.UNSTART;
  }
};

/**
 * Returns the effective display/action status of a document.
 *
 * Every status is derived from the real-time `ingestion_status` field
 * (the legacy `run` field is no longer returned).
 *
 * @param document - Document (or subset) carrying the raw status field.
 * @returns The `RunningStatus` to drive icons, dots and labels.
 *
 * @example
 * getDocumentRunningStatus({ ingestion_status: IngestionTaskStatus.SCHEDULED });
 * // => RunningStatus.QUEUED
 */
export const getDocumentRunningStatus = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
): RunningStatus => ingestionStatusToRunningStatus(document.ingestion_status);

/** Returns the message shown for a document's current run. */
export const getDocumentProgressMessage = (
  document: Pick<IDocumentInfo, 'latest_ingestion_event'>,
) => document.latest_ingestion_event?.message || '-';

/**
 * Whether a cancel request is currently in flight for the document.
 * Only the Go backend reports STOPPING; Python always returns false.
 *
 * @param document - Document (or subset) carrying `ingestion_status`.
 * @returns `true` while the cancel button must stay disabled.
 *
 * @example
 * isDocumentStopping({ ingestion_status: IngestionTaskStatus.STOPPING });
 * // => true
 */
export const isDocumentStopping = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
) => document.ingestion_status === IngestionTaskStatus.STOPPING;

// Go: ingestion_status is the only source of truth after the run field
// was removed. Active statuses cover the queued -> running -> stopping
// lifecycle; everything else (including UNSTART and missing status) is
// terminal/idle so polling stops and row actions become available.
const isGoDocumentProcessing = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
) =>
  !!document.ingestion_status &&
  activeIngestionStatuses.has(document.ingestion_status);

/**
 * Whether a document is currently being parsed (queued, running or
 * canceling). Drives the 5s list polling, disabled row actions and the
 * bulk-delete protection.
 *
 * @param document - Document (or subset) carrying the raw status field.
 * @returns `true` while any parse-related task is in progress.
 *
 * @example
 * isDocumentProcessing({ ingestion_status: IngestionTaskStatus.RUNNING });
 * // => true
 */
export const isDocumentProcessing = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
) => isGoDocumentProcessing(document);

// --- Parser prerequisite checks -------------------------------------------
// A file cannot be parsed under the dataset's current Parser operator when
// the operator does not declare the file's type family at all
// (unsupportedType), or when the family is declared but the model it needs
// is not set (missingModel): audio reads its per-setup ASR model, while
// video reads the Parser-level vision enhancement (enable_vision_enhancement
// + vlm.llm_id) — its model no longer lives on the family setup. The tenant
// default is deliberately not consulted: a global default does not make the
// file parsable here. Image files only need their family declared: the image
// OCR switch runs local extraction independently, and the vision model is a
// global supplement for it.
// These helpers power the upload warning and the parse-click validation.

export type ParserModelGap = {
  reason: ParserGapReason.MissingModel;
  fileType: FileType;
  modelKind: ParserModelKind.Asr | ParserModelKind.Vision;
};

export type UnsupportedTypeGap = {
  reason: ParserGapReason.UnsupportedType;
  fileType: FileType;
};

export type ParserGap = ParserModelGap | UnsupportedTypeGap;

export type FileParserGap = ParserGap & { name: string };

export const hasUnsupportedTypeGap = (gaps: ParserGap[]) =>
  gaps.some((gap) => gap.reason === ParserGapReason.UnsupportedType);

// Modal copy keyed by gap kind: missing models steer to adding the model,
// unsupported types to reselecting the parse method.
export function pickByGapKind<T>(
  gaps: ParserGap[],
  options: { missingModel: T; unsupportedType: T },
): T {
  return hasUnsupportedTypeGap(gaps)
    ? options.unsupportedType
    : options.missingModel;
}

type ParserSetup = Record<string, any> & { fileFormat?: string };

/**
 * The Parser-level vision enhancement globals (enable_vision_enhancement +
 * vlm.llm_id): one switch and model serve every media family. Undefined marks
 * a config saved before the model moved off the per-family setups — those
 * still carry the video model on the video setup itself.
 */
export type ParserVisionEnhancement = {
  enabled: boolean;
  modelId: string;
};

export type SavedParserConfig = {
  setups: ParserSetup[];
  visionEnhancement?: ParserVisionEnhancement;
};

const ExtensionToFileTypeMap: Record<string, FileType> = Object.entries(
  FileTypeSuffixMap,
).reduce<Record<string, FileType>>((acc, [fileType, suffixes]) => {
  for (const suffix of suffixes) {
    acc[suffix] = fileType as FileType;
  }
  return acc;
}, {});

export function getFileTypeByExtension(
  extension: string,
): FileType | undefined {
  return ExtensionToFileTypeMap[extension.toLowerCase()];
}

// Reads the Parser-level vision globals from a saved Parser entry. A boolean
// enable_vision_enhancement marks the current contract; configs saved before
// the model moved to the top level do not have it, and their video setup
// still holds the model — so the absence is meaningful, not a default-off.
function readVisionEnhancement(
  parserEntry: unknown,
): ParserVisionEnhancement | undefined {
  if (typeof parserEntry !== 'object' || parserEntry === null) {
    return undefined;
  }
  const params = parserEntry as Record<string, any>;
  const enabled = params.enable_vision_enhancement;
  if (typeof enabled !== 'boolean') {
    return undefined;
  }
  const modelId = params.vlm?.llm_id;
  return { enabled, modelId: typeof modelId === 'string' ? modelId : '' };
}

/**
 * Parser config exactly as saved: the per-family setups plus the Parser-level
 * vision enhancement globals. Accepts either a dataset or a document row: a
 * document's parser_config snapshots the parser it actually runs with
 * (dataset config at upload, the override after a parser change). Returns
 * null when parser_config has no Parser operator entry (legacy flat-shape
 * configs, pipelines without a Parser stage): type support cannot be
 * determined then and validation is skipped.
 */
export function getSavedParserConfig(
  source: { parser_config?: unknown } | null | undefined,
): SavedParserConfig | null {
  const parserConfig = source?.parser_config as Record<string, any> | undefined;

  const parserEntry = Object.entries(parserConfig ?? {}).find(
    ([operatorId]) => getOperatorType(operatorId) === Operator.Parser,
  )?.[1];

  if (!parserEntry) {
    return null;
  }
  return {
    setups: transformParserConfigSetups(parserEntry),
    visionEnhancement: readVisionEnhancement(parserEntry),
  };
}

export function findParserGap(
  fileType: FileType | undefined,
  config: SavedParserConfig,
): ParserGap | null {
  if (!fileType) {
    return null;
  }

  const setup = config.setups.find((x) => x.fileFormat === fileType);
  if (!setup) {
    return { reason: ParserGapReason.UnsupportedType, fileType };
  }

  switch (fileType) {
    case FileType.Audio: {
      // Audio keeps a per-setup model: it is an ASR model, not the shared
      // vision one.
      return setup.vlm?.llm_id
        ? null
        : {
            reason: ParserGapReason.MissingModel,
            fileType,
            modelKind: ParserModelKind.Asr,
          };
    }
    case FileType.Video: {
      const hasVisionModel = config.visionEnhancement
        ? config.visionEnhancement.enabled && !!config.visionEnhancement.modelId
        : !!setup.vlm?.llm_id;
      return hasVisionModel
        ? null
        : {
            reason: ParserGapReason.MissingModel,
            fileType,
            modelKind: ParserModelKind.Vision,
          };
    }
    default:
      return null;
  }
}

export function findFilesParserGaps(
  names: string[],
  config: SavedParserConfig,
): FileParserGap[] {
  return names.flatMap((name) => {
    const fileType = getFileTypeByExtension(getExtension(name));
    const gap = findParserGap(fileType, config);
    return gap ? [{ ...gap, name }] : [];
  });
}

/**
 * Per-document parse validation: each document row carries the parser config
 * it actually runs with (dataset snapshot at upload, the override after a
 * parser change), so it is validated against its own config. Rows without a
 * Parser entry fall back to `fallbackConfig` (the dataset-level config).
 */
export function findDocumentsParserGaps(
  documents: Array<{ name: string; parser_config?: unknown }>,
  fallbackConfig: SavedParserConfig | null,
): FileParserGap[] {
  return documents.flatMap((doc) => {
    const config = getSavedParserConfig(doc) ?? fallbackConfig;
    return config ? findFilesParserGaps([doc.name], config) : [];
  });
}
