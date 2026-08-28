import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import {
  FileType,
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

// Static parse-method values across all file types. With shouldUnregister the
// parse_method field is re-seeded from the node's saved form data when its
// widget remounts on a file-type switch, so it can hold a previous file type's
// static value. LLM model ids from the model tree are never in this set, so a
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
