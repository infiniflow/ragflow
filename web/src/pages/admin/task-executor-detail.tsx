import dayjs from 'dayjs';
import { isPlainObject } from 'lodash';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';

import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Rectangle,
  ResponsiveContainer,
  Tooltip,
  XAxis,
} from 'recharts';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

import { ScrollArea } from '@/components/ui/scroll-area';
import { formatDate, formatTime } from '@/utils/date';
import ServiceDetail from './service-detail';

interface TaskExecutorDetailProps {
  content?: AdminService.TaskExecutorInfo;
}

function CustomTooltip({ active, payload, ...restProps }: any) {
  if (active && payload && payload.length) {
    const item = payload.at(0)?.payload ?? {};

    return (
      <Card className="border bg-bg-base overflow-hidden shadow-xl">
        <CardHeader className="px-3 py-2 text-text-primary bg-bg-title">
          <CardTitle className="text-lg">
            {formatDate(restProps.label)}
          </CardTitle>
        </CardHeader>

        <CardContent className="p-0">
          <ScrollArea className="px-3">
            <div className="max-h-56">
              <JsonView
                src={item}
                displaySize={30}
                className="py-2 break-words text-text-secondary"
              />
            </div>
          </ScrollArea>
        </CardContent>
      </Card>
    );
  }

  return null;
}

function CustomAxisTick({ x, y, payload }: any) {
  return (
    <g transform={`translate(${x},${y})`}>
      <text
        x={0}
        y={0}
        dy={8}
        transform="rotate(60)"
        textAnchor="start"
        fill="rgb(var(--text-secondary))"
      >
        {formatTime(payload.value)}
      </text>
    </g>
  );
}

function TaskExecutorDetail({ content }: TaskExecutorDetailProps) {
  if (!isPlainObject(content)) {
    return <ServiceDetail content={content} />;
  }

  return (
    <section className="space-y-8">
      {Object.entries(content ?? {}).map(([name, data]) => {
        console.log(data);

        const items = data.map((x) => ({
          ...x,
          now: dayjs(x.now).valueOf(),
        }));

        const lastItem = items.at(-1);

        return (
          <Card key={name} className="border-0">
            <CardHeader className="text-text-primary">
              <CardTitle>{name}</CardTitle>

              <div className="text-sm text-text-secondary space-x-4">
                <span>Lag: {lastItem?.lag}</span>
                <span>Pending: {lastItem?.pending}</span>
              </div>
            </CardHeader>

            <CardContent>
              <ResponsiveContainer className="min-h-40 w-full">
                <BarChart
                  data={items}
                  margin={{ bottom: 32 }}
                  className="text-sm text-text-secondary"
                >
                  <XAxis
                    dataKey="now"
                    type="number"
                    scale="time"
                    domain={['dataMin', 'dataMax']}
                    tick={CustomAxisTick}
                    interval="equidistantPreserveStart"
                    angle={60}
                    minTickGap={16}
                    allowDataOverflow
                    padding={{ left: 24, right: 24 }}
                  />

                  {/* <YAxis
                      type="number"
                      tick={{ fill: 'rgb(var(--text-secondary))' }}
                    /> */}

                  <CartesianGrid
                    strokeDasharray="3 3"
                    className="stroke-border-default"
                  />

                  <Tooltip
                    trigger="click"
                    content={CustomTooltip}
                    wrapperStyle={{
                      pointerEvents: 'auto',
                    }}
                  />

                  <Legend wrapperStyle={{ bottom: 0 }} />

                  <Bar
                    dataKey="done"
                    fill="rgb(var(--state-success))"
                    activeBar={
                      <Rectangle
                        fill="rgb(var(--state-success))"
                        stroke="var(--bg-base)"
                      />
                    }
                  />

                  <Bar
                    dataKey="failed"
                    fill="rgb(var(--state-error))"
                    activeBar={
                      <Rectangle
                        fill="rgb(var(--state-error))"
                        stroke="var(--bg-base)"
                      />
                    }
                  />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        );
      })}
    </section>
  );
}

export default TaskExecutorDetail;
