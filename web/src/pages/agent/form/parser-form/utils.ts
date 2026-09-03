import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import {
  FileType,
  FileTypeDefaultModelFieldMap,
  ImageParseMethod,
  initialParserValues,
} from '../../constant/pipeline';

export function buildFieldNameWithPrefix(name: string, prefix: string) {
  return `${prefix}.${name}`;
}

export function withDefaultParserModels(
  formValues: Record<string, any>,
  defaultModelDictionary: Record<string, string>,
) {
  const setups = formValues?.setups;
  if (!Array.isArray(setups)) {
    return formValues;
  }

  return {
    ...formValues,
    setups: setups.map((setup) => {
      const field = FileTypeDefaultModelFieldMap[setup?.fileFormat as FileType];
      const modelId = field ? defaultModelDictionary[field] : '';
      if (!modelId || setup?.vlm?.llm_id) {
        return setup;
      }
      return { ...setup, vlm: { ...setup?.vlm, llm_id: modelId } };
    }),
  };
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
