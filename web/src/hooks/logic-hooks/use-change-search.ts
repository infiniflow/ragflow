import { useCallback, useState } from 'react';

export const useHandleSearchStrChange = () => {
  const [searchString, setSearchString] = useState('');
  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const value = e.target.value;
      setSearchString(value);
    },
    [],
  );

  return { handleInputChange, searchString };
};
