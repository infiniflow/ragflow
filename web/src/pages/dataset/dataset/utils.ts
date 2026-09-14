import { Operator } from '@/constants/agent';
import type { IDataset } from '@/interfaces/database/dataset';
import type { IDocumentInfo } from '@/interfaces/database/document';
import {
  FileType,
  FileTypeSuffixMap,
  initialParserValues,
} from '@/pages/agent/constant/pipeline';
import { isStaticParseMethod } from '@/pages/agent/form/parser-form/utils';
import { pickByBackend } from '@/utils/backend-variant';
import { getExtension } from '@/utils/document-util';
import {
  getOperatorType,
  transformParserConfigSetups,
} from '@/utils/pipeline-operator';
import { cloneDeep } from 'lodash';
import { IngestionTaskStatus, RunningStatus } from './constant';

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
const isPythonDocumentProcessing = (
  document: Pick<IDocumentInfo, 'run'>,
) => isParserRunning(document.run);

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

// --- Parser model prerequisite checks -------------------------------------
// Audio/video/image files can only be parsed when the matching model is
// configured on the dataset's Parser operator. The tenant default is
// deliberately not consulted: parsing reads the operator setup, so a global
// default does not make the file parsable.
// These helpers power the upload warning and the parse-click validation.

export type ParserModelGap = {
  fileType: FileType;
  modelKind: 'asr' | 'vision';
};

export type FileModelGap = ParserModelGap & { name: string };

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
 * Effective parser setups for a dataset: the saved Parser operator entry
 * (stored as a flattened map keyed by file format) merged over the default
 * setups, so untouched file types validate against their defaults.
 */
export function getEffectiveParserSetups(
  knowledgeBase: Pick<IDataset, 'parser_config'> | null | undefined,
): ParserSetup[] {
  const parserConfig = knowledgeBase?.parser_config as
    | Record<string, any>
    | undefined;

  const parserEntry = Object.entries(parserConfig ?? {}).find(
    ([operatorId]) => getOperatorType(operatorId) === Operator.Parser,
  )?.[1];

  const savedSetups = transformParserConfigSetups(parserEntry);
  const savedByFileType = new Map(
    savedSetups.map((setup) => [setup.fileFormat, setup]),
  );

  const defaultSetups = cloneDeep(initialParserValues.setups) as ParserSetup[];
  const merged = defaultSetups.map(
    (setup) => savedByFileType.get(setup.fileFormat) ?? setup,
  );
  for (const setup of savedSetups) {
    if (!defaultSetups.some((x) => x.fileFormat === setup.fileFormat)) {
      merged.push(setup);
    }
  }
  return merged;
}

export function findMissingParserModel(
  fileType: FileType | undefined,
  setups: ParserSetup[],
): ParserModelGap | null {
  const setup = setups.find((x) => x.fileFormat === fileType);

  switch (fileType) {
    case FileType.Audio: {
      const configured = setup?.vlm?.llm_id;
      return configured ? null : { fileType, modelKind: 'asr' };
    }
    case FileType.Video: {
      const configured = setup?.vlm?.llm_id;
      return configured ? null : { fileType, modelKind: 'vision' };
    }
    case FileType.Image: {
      // The image parser's vision model is picked as parse_method; a static
      // method (e.g. ocr) means no vision model is configured.
      const configured =
        !!setup?.parse_method && !isStaticParseMethod(setup.parse_method);
      return configured ? null : { fileType, modelKind: 'vision' };
    }
    default:
      return null;
  }
}

export function findFilesMissingParserModels(
  names: string[],
  setups: ParserSetup[],
): FileModelGap[] {
  return names.flatMap((name) => {
    const fileType = getFileTypeByExtension(getExtension(name));
    const gap = findMissingParserModel(fileType, setups);
    return gap ? [{ ...gap, name }] : [];
  });
}
