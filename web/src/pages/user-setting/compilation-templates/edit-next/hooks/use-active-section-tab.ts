import { useEffect, useState } from 'react';

export const useActiveSectionTab = (sectionNames: string[]) => {
  const [activeSectionTab, setActiveSectionTab] = useState<string>('');

  useEffect(() => {
    setActiveSectionTab((prev) =>
      sectionNames.includes(prev) ? prev : (sectionNames[0] ?? ''),
    );
  }, [sectionNames]);

  return { activeSectionTab, setActiveSectionTab };
};
