import { useSelectKnowledgeList } from '@/hooks/knowledgeHook';
import { useState } from 'react';

export const useSearchKnowledge = () => {
  const [searchString, setSearchString] = useState<string>('');

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchString(e.target.value);
  };
  return {
    searchString,
    handleInputChange,
  };
};

export const useSelectKnowledgeListByKeywords = (keywords: string) => {
  const list = useSelectKnowledgeList();
  return list.filter((x) => x.name.includes(keywords));
};
