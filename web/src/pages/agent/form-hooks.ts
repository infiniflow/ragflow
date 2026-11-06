import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';

export const useBuildSortOptions = () => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['date', 'relevance'].map((x) => ({
      value: x,
      label: x === 'date' ? t(x, { defaultValue: 'Date' }) : t(x),
    }));
  }, [t]);
  return options;
};
