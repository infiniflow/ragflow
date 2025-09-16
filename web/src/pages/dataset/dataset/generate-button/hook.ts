import { useQuery } from '@tanstack/react-query';
import { useCallback } from 'react';
export const generateStatus = {
  running: 'running',
  completed: 'completed',
  start: 'start',
};
const useFetchGenerateData = () => {
  let number = 10;
  // TODO: 获取数据
  const { data, isFetching: loading } = useQuery({
    queryKey: ['generateData', 'id'],
    initialData: { id: 0, percent: 0, type: 'running' },
    gcTime: 0,
    refetchInterval: 3000,
    queryFn: async () => {
      number += Math.random() * 10;
      const data = {
        id: Math.random(),
        percent: number,
        type: generateStatus.running,
      };
      return data;
    },
  });
  const pauseGenerate = useCallback(() => {
    // TODO: pause generate
    console.log('pause generate');
  }, []);
  return { data, loading, pauseGenerate };
};
export { useFetchGenerateData };
