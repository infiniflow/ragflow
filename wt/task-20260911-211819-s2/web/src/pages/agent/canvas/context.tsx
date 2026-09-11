import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useRef,
} from 'react';

interface DropdownContextType {
  canShowDropdown: () => boolean;
  setActiveDropdown: (type: 'handle' | 'drag') => void;
  clearActiveDropdown: () => void;
}

const DropdownContext = createContext<DropdownContextType | null>(null);

export const useDropdownManager = () => {
  const context = useContext(DropdownContext);
  if (!context) {
    throw new Error('useDropdownManager must be used within DropdownProvider');
  }
  return context;
};

interface DropdownProviderProps {
  children: ReactNode;
}

export const DropdownProvider = ({ children }: DropdownProviderProps) => {
  const activeDropdownRef = useRef<'handle' | 'drag' | null>(null);

  const canShowDropdown = useCallback(() => {
    const current = activeDropdownRef.current;
    return !current;
  }, []);

  const setActiveDropdown = useCallback((type: 'handle' | 'drag') => {
    activeDropdownRef.current = type;
  }, []);

  const clearActiveDropdown = useCallback(() => {
    activeDropdownRef.current = null;
  }, []);

  const value: DropdownContextType = {
    canShowDropdown,
    setActiveDropdown,
    clearActiveDropdown,
  };

  return (
    <DropdownContext.Provider value={value}>
      {children}
    </DropdownContext.Provider>
  );
};
