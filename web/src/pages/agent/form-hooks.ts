import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';

export const useBuildSortOptions = () => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['data', 'relevance'].map((x) => ({
      value: x,
      label: t(x),
    }));
  }, [t]);
  return options;
};
