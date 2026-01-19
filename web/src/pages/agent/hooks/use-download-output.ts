import { useFetchAgent } from '@/hooks/use-agent-request';
import { ITraceData } from '@/interfaces/database/agent';
import { downloadJsonFile } from '@/utils/file-util';
import { get, isEmpty } from 'lodash';
import { useCallback } from 'react';

export function findEndOutput(list?: ITraceData[]) {
  if (Array.isArray(list)) {
    const trace = list.find((x) => x.component_id === 'END')?.trace;

    const str = get(trace, '0.message');

    try {
      if (!isEmpty(str)) {
        const json = JSON.parse(str);
        return json;
      }
    } catch (error) {}
  }
}

export function isEndOutputEmpty(list?: ITraceData[]) {
  return isEmpty(findEndOutput(list));
}
export function useDownloadOutput(data?: ITraceData[]) {
  const { data: agent } = useFetchAgent();

  const handleDownloadJson = useCallback(() => {
    const output = findEndOutput(data);
    if (!isEndOutputEmpty(data)) {
      downloadJsonFile(output, `${agent.title}.json`);
    }
  }, [agent.title, data]);

  return {
    handleDownloadJson,
  };
}
