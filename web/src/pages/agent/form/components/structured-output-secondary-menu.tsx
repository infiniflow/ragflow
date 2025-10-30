import { JSONSchema } from '@/components/jsonjoy-builder';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { cn } from '@/lib/utils';
import { get, isPlainObject } from 'lodash';
import { ChevronRight } from 'lucide-react';
import { PropsWithChildren, ReactNode, useCallback } from 'react';

type DataItem = { label: ReactNode; value: string; parentLabel?: ReactNode };

type StructuredOutputSecondaryMenuProps = {
  data: DataItem;
  click(option: { label: ReactNode; value: string }): void;
  filteredStructuredOutput: JSONSchema;
} & PropsWithChildren;
export function StructuredOutputSecondaryMenu({
  data,
  click,
  filteredStructuredOutput,
}: StructuredOutputSecondaryMenuProps) {
  const renderAgentStructuredOutput = useCallback(
    (values: any, option: { label: ReactNode; value: string }) => {
      if (isPlainObject(values) && 'properties' in values) {
        return (
          <ul className="border-l">
            {Object.entries(values.properties).map(([key, value]) => {
              const nextOption = {
                label: option.label + `.${key}`,
                value: option.value + `.${key}`,
              };

              const dataType = get(value, 'type');

              return (
                <li key={key} className="pl-1">
                  <div
                    onClick={() => click(nextOption)}
                    className="hover:bg-bg-card p-1 text-text-primary rounded-sm flex justify-between"
                  >
                    {key}
                    <span className="text-text-secondary">{dataType}</span>
                  </div>
                  {dataType === 'object' &&
                    renderAgentStructuredOutput(value, nextOption)}
                </li>
              );
            })}
          </ul>
        );
      }

      return <div></div>;
    },
    [click],
  );

  return (
    <HoverCard key={data.value} openDelay={100} closeDelay={100}>
      <HoverCardTrigger asChild>
        <li className="hover:bg-bg-card py-1 px-2 text-text-primary rounded-sm text-sm flex justify-between items-center">
          {data.label} <ChevronRight className="size-3.5 text-text-secondary" />
        </li>
      </HoverCardTrigger>
      <HoverCardContent
        side="left"
        align="start"
        className={cn(
          'min-w-[140px]  border border-border rounded-md shadow-lg p-0',
        )}
      >
        <section className="p-2">
          <div className="p-1">{data?.parentLabel} structured output:</div>
          {renderAgentStructuredOutput(filteredStructuredOutput, data)}
        </section>
      </HoverCardContent>
    </HoverCard>
  );
}
