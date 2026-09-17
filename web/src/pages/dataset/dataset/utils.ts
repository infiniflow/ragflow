import { Operator } from '@/constants/agent';
import { FileType, FileTypeSuffixMap } from '@/constants/file';
import type { IDocumentInfo } from '@/interfaces/database/document';
import { pickByBackend } from '@/utils/backend-variant';
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

export const isParserRunning = (text?: RunningStatus) => {
  const isRunning = text === RunningStatus.RUNNING;
  return isRunning;
};

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
 * Go derives every status from the real-time `ingestion_status` field
 * (the `run` field is no longer returned). Python keeps reading the
 * legacy `run` field, which is always present on Python responses.
 *
 * @param document - Document (or subset) carrying the raw status fields.
 * @returns The `RunningStatus` to drive icons, dots and labels.
 *
 * @example
 * // Go backend
 * getDocumentRunningStatus({ ingestion_status: IngestionTaskStatus.SCHEDULED });
 * // => RunningStatus.QUEUED
 *
 * @example
 * // Python backend
 * getDocumentRunningStatus({ run: RunningStatus.RUNNING });
 * // => RunningStatus.RUNNING
 */
export const getDocumentRunningStatus = (
  document: Pick<IDocumentInfo, 'run' | 'ingestion_status'>,
): RunningStatus =>
  pickByBackend({
    go: ingestionStatusToRunningStatus(document.ingestion_status),
    python: document.run ?? RunningStatus.UNSTART,
  });

/** Returns the backend-specific message shown for a document's current run. */
export const getDocumentProgressMessage = (
  document: Pick<IDocumentInfo, 'progress_msg' | 'latest_ingestion_event'>,
) =>
  pickByBackend({
    go: document.latest_ingestion_event?.message,
    python: document.progress_msg,
  }) || '-';

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

// Python: ingestion_status is never present on Python responses (the
// Python backend only serializes the legacy run field), so run is the
// only signal — exactly the pre-Go polling contract
// (docs.some(doc => doc.run === RUNNING)).
const isPythonDocumentProcessing = (document: Pick<IDocumentInfo, 'run'>) =>
  isParserRunning(document.run);

/**
 * Whether a document is currently being parsed (queued, running or
 * canceling). Drives the 5s list polling, disabled row actions and the
 * bulk-delete protection.
 *
 * The check is backend-specific: on Go it relies solely on
 * `ingestion_status`, on Python it follows the legacy `run`-based logic.
 *
 * @param document - Document (or subset) carrying the raw status fields.
 * @returns `true` while any parse-related task is in progress.
 *
 * @example
 * // Go backend
 * isDocumentProcessing({ ingestion_status: IngestionTaskStatus.RUNNING });
 * // => true
 *
 * @example
 * // Python backend
 * isDocumentProcessing({ run: RunningStatus.DONE });
 * // => false
 */
export const isDocumentProcessing = (
  document: Pick<IDocumentInfo, 'run' | 'ingestion_status'>,
) =>
  pickByBackend({
    go: isGoDocumentProcessing(document),
    python: isPythonDocumentProcessing(document),
  });

// --- Parser prerequisite checks -------------------------------------------
// A file cannot be parsed under the dataset's current Parser operator when
// the operator does not declare the file's type family at all
// (unsupportedType), or — for audio/video — when the declared setup carries
// no model (missingModel). The tenant default is deliberately not consulted:
// parsing reads the operator setup, so a global default does not make the
// file parsable. Image files only need their family declared: the image
// parser always runs OCR and merely supplements it with the vision model
// (picked as the image parse_method, with the tenant default as fallback).
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

type ParserSetup = Record<string, any> & { fileFormat?: string };

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

/**
 * Parser setups exactly as saved, keyed by file format — the keys are the
 * type families the pipeline/built-in method declares. Accepts either a
 * dataset or a document row: a document's parser_config snapshots the parser
 * it actually runs with (dataset config at upload, the override after a
 * parser change). Returns null when parser_config has no Parser operator
 * entry (legacy flat-shape configs, pipelines without a Parser stage): type
 * support cannot be determined then and validation is skipped.
 */
export function getSavedParserSetups(
  source: { parser_config?: unknown } | null | undefined,
): ParserSetup[] | null {
  const parserConfig = source?.parser_config as Record<string, any> | undefined;

  const parserEntry = Object.entries(parserConfig ?? {}).find(
    ([operatorId]) => getOperatorType(operatorId) === Operator.Parser,
  )?.[1];

  if (!parserEntry) {
    return null;
  }
  return transformParserConfigSetups(parserEntry);
}

export function findParserGap(
  fileType: FileType | undefined,
  setups: ParserSetup[],
): ParserGap | null {
  if (!fileType) {
    return null;
  }

  const setup = setups.find((x) => x.fileFormat === fileType);
  if (!setup) {
    return { reason: ParserGapReason.UnsupportedType, fileType };
  }

  switch (fileType) {
    case FileType.Audio: {
      return setup.vlm?.llm_id
        ? null
        : {
            reason: ParserGapReason.MissingModel,
            fileType,
            modelKind: ParserModelKind.Asr,
          };
    }
    case FileType.Video: {
      return setup.vlm?.llm_id
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
  setups: ParserSetup[],
): FileParserGap[] {
  return names.flatMap((name) => {
    const fileType = getFileTypeByExtension(getExtension(name));
    const gap = findParserGap(fileType, setups);
    return gap ? [{ ...gap, name }] : [];
  });
}

/**
 * Per-document parse validation: each document row carries the parser config
 * it actually runs with (dataset snapshot at upload, the override after a
 * parser change), so it is validated against its own setups. Rows without a
 * Parser entry fall back to `fallbackSetups` (the dataset-level config).
 */
export function findDocumentsParserGaps(
  documents: Array<{ name: string; parser_config?: unknown }>,
  fallbackSetups: ParserSetup[] | null,
): FileParserGap[] {
  return documents.flatMap((doc) => {
    const setups = getSavedParserSetups(doc) ?? fallbackSetups;
    return setups ? findFilesParserGaps([doc.name], setups) : [];
  });
}
