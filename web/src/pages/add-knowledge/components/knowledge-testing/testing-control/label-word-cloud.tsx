import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { Chart } from '@antv/g2';
import { useCallback, useEffect, useMemo, useRef } from 'react';

export function LabelWordCloud() {
  const domRef = useRef<HTMLDivElement>(null);
  let chartRef = useRef<Chart>();
  const { labels } = useSelectTestingResult();

  const list = useMemo(() => {
    if (!labels) {
      return [];
    }

    return Object.keys(labels).reduce<
      Array<{ text: string; name: string; value: number }>
    >((pre, cur) => {
      pre.push({ name: cur, text: cur, value: labels[cur] });

      return pre;
    }, []);
  }, [labels]);

  const renderWordCloud = useCallback(() => {
    if (domRef.current && list.length) {
      chartRef.current = new Chart({ container: domRef.current });

      chartRef.current.options({
        type: 'wordCloud',
        autoFit: true,
        layout: {
          fontSize: [6, 15],
        },
        data: {
          type: 'inline',
          value: list,
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
  }, [list]);

  useEffect(() => {
    renderWordCloud();

    return () => {
      chartRef.current?.destroy();
    };
  }, [renderWordCloud]);

  return <div ref={domRef} className="w-full h-[13vh]"></div>;
}
