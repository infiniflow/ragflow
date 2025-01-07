import { useFetchTagList } from '@/hooks/knowledge-hooks';
import { Chart } from '@antv/g2';
import { useCallback, useEffect, useRef } from 'react';

export function TagWordCloud() {
  const domRef = useRef<HTMLDivElement>(null);
  let chartRef = useRef<Chart>();
  const { list } = useFetchTagList();

  const renderWordCloud = useCallback(() => {
    if (domRef.current) {
      chartRef.current = new Chart({ container: domRef.current });

      chartRef.current.options({
        type: 'wordCloud',
        autoFit: true,
        layout: { fontSize: [20, 100] },
        data: {
          type: 'inline',
          value: list.map((x) => ({ text: x[0], value: x[1], name: x[0] })),
        },
        encode: { color: 'text' },
        legend: false,
        tooltip: false,
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

  return <div ref={domRef} className="w-full h-[38vh]"></div>;
}
