import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { cloneDeep } from 'lodash';
import {
  FileType,
  FileTypeDefaultModelFieldMap,
  ImageParseMethod,
  initialParserValues,
} from '../../constant/pipeline';

export function buildFieldNameWithPrefix(name: string, prefix: string) {
  return `${prefix}.${name}`;
}

export function getInitialParseMethod(fileType: FileType): string {
  const setup = initialParserValues.setups.find(
    (x) => x.fileFormat === fileType,
  );
  return setup?.parse_method ?? '';
}

// Static parse-method values across all file types. Forms saved while the
// file type was still switchable can hold another file type's static value on
// parse_method. LLM model ids from the model tree are never in this set, so a
// user-picked model is never treated as foreign.
// Note: ParseDocumentType is a const enum — list members explicitly instead of
// Object.values, which is not allowed on const enums (TS2475).
const KnownStaticParseMethods = new Set<string>([
  ParseDocumentType.DeepDOC,
  ParseDocumentType.PlainText,
  ParseDocumentType.Docling,
  ParseDocumentType.OpenDataLoader,
  ParseDocumentType.TCADPParser,
  ImageParseMethod.OCR,
]);

export function isForeignParseMethod(
  fileType: FileType,
  value: unknown,
): value is string {
  return (
    typeof value === 'string' &&
    KnownStaticParseMethods.has(value) &&
    value !== getInitialParseMethod(fileType)
  );
}

export function isStaticParseMethod(value: unknown): value is string {
  return typeof value === 'string' && KnownStaticParseMethods.has(value);
}

// Builds the default setup for a file type being added to the parser form,
// prefilling the tenant default model for types that have one (video/audio).
export function buildInitialParserSetup(
  fileType: FileType,
  defaultModelDictionary: Record<string, string>,
) {
  const setup = initialParserValues.setups.find(
    (x) => x.fileFormat === fileType,
  );
  if (!setup) {
    return undefined;
  }

  const nextSetup = cloneDeep(setup) as Record<string, any>;
  const field = FileTypeDefaultModelFieldMap[fileType];
  const modelId = field ? defaultModelDictionary[field] : '';
  if (modelId) {
    nextSetup.vlm = { ...nextSetup.vlm, llm_id: modelId };
  }
  return nextSetup;
}

// Form values for a freshly added Parser node: every default file type with its
// tenant default model prefilled. Only applies to node creation — an existing
// form is shown as saved, so a cleared model stays cleared.
export function buildInitialParserValues(
  defaultModelDictionary: Record<string, string>,
) {
  return {
    ...initialParserValues,
    setups: initialParserValues.setups.map(
      (setup) =>
        buildInitialParserSetup(
          setup.fileFormat as FileType,
          defaultModelDictionary,
        ) ?? setup,
    ),
  };
}
