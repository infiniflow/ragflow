import { useSelectParserList } from '@/hooks/user-setting-hooks';
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
      'qa',
      'knowledge_graph',
    ],
  ],
  [
    ['doc', 'docx'],
    [
      'naive',
      'resume',
      'book',
      'laws',
      'one',
      'qa',
      'manual',
      'knowledge_graph',
    ],
  ],
  [
    ['xlsx', 'xls'],
    ['naive', 'qa', 'table', 'one', 'knowledge_graph'],
  ],
  [['ppt', 'pptx'], ['presentation']],
  [
    ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'tif', 'tiff', 'webp', 'svg', 'ico'],
    ['picture'],
  ],
  [
    ['txt'],
    [
      'naive',
      'resume',
      'book',
      'laws',
      'one',
      'qa',
      'table',
      'knowledge_graph',
    ],
  ],
  [
    ['csv'],
    [
      'naive',
      'resume',
      'book',
      'laws',
      'one',
      'qa',
      'table',
      'knowledge_graph',
    ],
  ],
  [['md'], ['naive', 'qa', 'knowledge_graph']],
  [['json'], ['naive', 'knowledge_graph']],
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

  useEffect(() => {
    setSelectedTag(parserId);
  }, [parserId, documentId]);

  const handleChange = (tag: string) => {
    // const nextSelectedTag = checked ? tag : selectedTag;
    setSelectedTag(tag);
  };

  return { parserList: nextParserList, handleChange, selectedTag };
};
