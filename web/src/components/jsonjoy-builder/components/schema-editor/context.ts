import React, { useContext } from 'react';
import { KeyInputProps } from './interface';

export const KeyInputContext = React.createContext<KeyInputProps>({});

export function useInputPattern() {
  const x = useContext(KeyInputContext);
  return x.pattern;
}
