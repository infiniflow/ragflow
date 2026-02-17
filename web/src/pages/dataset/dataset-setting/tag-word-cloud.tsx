import { useFetchTagList } from '@/hooks/use-knowledge-request';
import { Chart } from '@antv/g2';
import { sumBy } from 'lodash';
import { useCallback, useEffect, useMemo, useRef } from 'react';

export function TagWordCloud() {
  const domRef = useRef<HTMLDivElement>(null);
  let chartRef = useRef<Chart>();
  const { list } = useFetchTagList();

  const { list: tagList } = useMemo(() => {
    const nextList = list.sort((a, b) => b[1] - a[1]).slice(0, 256);

    return {
      list: nextList.map((x) => ({ text: x[0], value: x[1], name: x[0] })),
      sumValue: sumBy(nextList, (x: [string, number]) => x[1]),
      length: nextList.length,
    };
  }, [list]);

  const renderWordCloud = useCallback(() => {
    if (domRef.current) {
      chartRef.current = new Chart({ container: domRef.current });

      chartRef.current.options({
        type: 'wordCloud',
        autoFit: true,
        layout: {
          fontSize: [10, 50],
          // fontSize: (d: any) => {
          //   if (d.value) {
          //     return (d.value / sumValue) * 100 * (length / 10);
          //   }
          //   return 0;
          // },
        },
        data: {
          type: 'inline',
          value: tagList,
        },
        encode: { color: 'text' },
        legend: false,
        tooltip: {
          title: 'name', // title
          items: ['value'], // data item
        },
      });

      chartRef.current.render();
    }
  }, [tagList]);

  useEffect(() => {
    renderWordCloud();

    return () => {
      chartRef.current?.destroy();
    };
  }, [renderWordCloud]);

  return <div ref={domRef} className="w-full h-[38vh]"></div>;
}
