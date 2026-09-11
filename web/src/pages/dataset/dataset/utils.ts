import { Operator } from '@/constants/agent';
import type { IDataset } from '@/interfaces/database/dataset';
import type { IDocumentInfo } from '@/interfaces/database/document';
import {
  FileType,
  FileTypeSuffixMap,
  initialParserValues,
} from '@/pages/agent/constant/pipeline';
import { isStaticParseMethod } from '@/pages/agent/form/parser-form/utils';
import { getExtension } from '@/utils/document-util';
import {
  getOperatorType,
  transformParserConfigSetups,
} from '@/utils/pipeline-operator';
import { cloneDeep } from 'lodash';
import { IngestionTaskStatus, RunningStatus } from './constant';

export const isParserRunning = (text: RunningStatus) => {
  const isRunning =
    text === RunningStatus.RUNNING || text === RunningStatus.SCHEDULE;
  return isRunning;
};

export const isDocumentQueued = (
  document: Pick<IDocumentInfo, 'ingestion_status'>,
) =>
  document.ingestion_status === IngestionTaskStatus.CREATED ||
  document.ingestion_status === IngestionTaskStatus.SCHEDULED;

// Go ingestion status can advance before the legacy document run field. The
// Python endpoint omits ingestion_status, so run remains the compatibility path.
// A terminal legacy run status is authoritative: the Go backend may leave
// ingestion_status at STOPPING after a cancel completes, and the document must
// then be treated as not running so its parsing style and restart action work.
export const isDocumentProcessing = (
  document: Pick<IDocumentInfo, 'run' | 'ingestion_status'>,
) => {
  if (isParserRunning(document.run)) return true;
  if (
    document.run === RunningStatus.CANCEL ||
    document.run === RunningStatus.DONE ||
    document.run === RunningStatus.FAIL
  ) {
    return false;
  }
  return (
    document.ingestion_status === IngestionTaskStatus.CREATED ||
    document.ingestion_status === IngestionTaskStatus.SCHEDULED ||
    document.ingestion_status === IngestionTaskStatus.RUNNING ||
    document.ingestion_status === IngestionTaskStatus.STOPPING
  );
};

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
