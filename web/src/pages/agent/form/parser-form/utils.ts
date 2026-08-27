import { FileType, initialParserValues } from '../../constant/pipeline';

export function buildFieldNameWithPrefix(name: string, prefix: string) {
  return `${prefix}.${name}`;
}

export function getInitialParseMethod(fileType: FileType): string {
  const setup = initialParserValues.setups.find(
    (x) => x.fileFormat === fileType,
  );
  return setup?.parse_method ?? '';
}
