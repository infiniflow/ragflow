import { IMemory } from '@/pages/memories/interface';
import React, { createContext, ReactNode, useContext } from 'react';

interface MemorySettingContextType {
  data: IMemory;
}

const MemorySettingContext = createContext<
  MemorySettingContextType | undefined
>(undefined);

export const MemorySettingProvider: React.FC<{
  children: ReactNode;
  data: IMemory;
}> = ({ children, data }) => {
  return (
    <MemorySettingContext.Provider value={{ data }}>
      {children}
    </MemorySettingContext.Provider>
  );
};

// eslint-disable-next-line react-refresh/only-export-components
export const useMemorySettingContext = (): MemorySettingContextType => {
  const context = useContext(MemorySettingContext);
  if (context === undefined) {
    throw new Error(
      'useMemorySettingContext must be used within a MemorySettingProvider',
    );
  }
  return context;
};
