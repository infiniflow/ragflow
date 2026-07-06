import { IDataset } from '@/interfaces/database/dataset';
import React, { createContext, ReactNode, useContext } from 'react';

interface KnowledgeBaseContextType {
  knowledgeBase: IDataset | null;
  loading: boolean;
}

const KnowledgeBaseContext = createContext<
  KnowledgeBaseContextType | undefined
>(undefined);

export const KnowledgeBaseProvider: React.FC<{
  children: ReactNode;
  knowledgeBase: IDataset | null;
  loading: boolean;
}> = ({ children, knowledgeBase, loading }) => {
  return (
    <KnowledgeBaseContext.Provider value={{ knowledgeBase, loading }}>
      {children}
    </KnowledgeBaseContext.Provider>
  );
};

// eslint-disable-next-line react-refresh/only-export-components
export const useKnowledgeBaseContext = (): KnowledgeBaseContextType => {
  const context = useContext(KnowledgeBaseContext);
  if (context === undefined) {
    throw new Error(
      'useKnowledgeBaseContext must be used within a KnowledgeBaseProvider',
    );
  }
  return context;
};
