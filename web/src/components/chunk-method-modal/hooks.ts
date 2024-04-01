import {
  useFetchTenantInfo,
  useSelectParserList,
} from '@/hooks/userSettingHook';
import { useEffect, useMemo, useState } from 'react';

const ParserListMap = new Map([
  [
    ['pdf'],
    [
      'naive',
      'resume',
      'manual',
      'paper',
      'book',
      'laws',
      'presentation',
      'one',
    ],
  ],
  [
    ['doc', 'docx'],
    ['naive', 'resume', 'book', 'laws', 'one'],
  ],
  [
    ['xlsx', 'xls'],
    ['naive', 'qa', 'table', 'one'],
  ],
  [['ppt', 'pptx'], ['presentation']],
  [
    ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'tif', 'tiff', 'webp', 'svg', 'ico'],
    ['picture'],
  ],
  [['txt'], ['naive', 'resume', 'book', 'laws', 'one', 'qa', 'table']],
  [['csv'], ['naive', 'resume', 'book', 'laws', 'one', 'qa', 'table']],
]);

const getParserList = (
  values: string[],
  parserList: Array<{
    value: string;
    label: string;
  }>,
) => {
  return parserList.filter((x) => values?.some((y) => y === x.value));
};

export const useFetchParserListOnMount = (
  documentId: string,
  parserId: string,
  documentExtension: string,
) => {
  const [selectedTag, setSelectedTag] = useState('');
  const parserList = useSelectParserList();

  const nextParserList = useMemo(() => {
    const key = [...ParserListMap.keys()].find((x) =>
      x.some((y) => y === documentExtension),
    );
    if (key) {
      const values = ParserListMap.get(key);
      return getParserList(values ?? [], parserList);
    }

    return getParserList(
      ['naive', 'resume', 'book', 'laws', 'one', 'qa', 'table'],
      parserList,
    );
  }, [parserList, documentExtension]);

  useFetchTenantInfo();

  useEffect(() => {
    setSelectedTag(parserId);
  }, [parserId, documentId]);

  const handleChange = (tag: string, checked: boolean) => {
    const nextSelectedTag = checked ? tag : selectedTag;
    setSelectedTag(nextSelectedTag);
  };

  return { parserList: nextParserList, handleChange, selectedTag };
};
