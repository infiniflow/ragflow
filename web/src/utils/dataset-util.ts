import { DocumentParserType } from '@/constants/knowledge';

export function isKnowledgeGraphParser(parserId: DocumentParserType) {
  return parserId === DocumentParserType.KnowledgeGraph;
}

export function isNaiveParser(parserId: DocumentParserType) {
  return parserId === DocumentParserType.Naive;
}

export type FilterType = {
  id: string;
  label: string;
  count: number;
};

export function groupListByType<T extends Record<string, any>>(
  list: T[],
  idField: string,
  labelField: string,
) {
  const fileTypeList: FilterType[] = [];
  list.forEach((x) => {
    const item = fileTypeList.find((y) => y.id === x[idField]);
    if (!item) {
      fileTypeList.push({ id: x[idField], label: x[labelField], count: 1 });
    } else {
      item.count += 1;
    }
  });

  return fileTypeList;
}
