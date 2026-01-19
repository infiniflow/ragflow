import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { cn } from '@/lib/utils';
import { getStructuredDatatype } from '@/utils/canvas-util';
import { get, isEmpty, isPlainObject } from 'lodash';
import { ChevronRight } from 'lucide-react';
import { PropsWithChildren, ReactNode, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { JsonSchemaDataType } from '../../constant';
import { useGetStructuredOutputByValue } from '../../hooks/use-build-structured-output';
import { hasSpecificTypeChild } from '../../utils/filter-agent-structured-output';

type DataItem = { label: ReactNode; value: string; parentLabel?: ReactNode };

type StructuredOutputSecondaryMenuProps = {
  data: DataItem;
  click(option: { label: ReactNode; value: string }): void;
  types?: JsonSchemaDataType[];
} & PropsWithChildren;

export function StructuredOutputSecondaryMenu({
  data,
  click,
  types = [],
}: StructuredOutputSecondaryMenuProps) {
  const { t } = useTranslation();
  const filterStructuredOutput = useGetStructuredOutputByValue();
  const structuredOutput = filterStructuredOutput(data.value);

  const handleSubMenuClick = useCallback(
    (option: { label: ReactNode; value: string }, dataType?: string) => () => {
      // The query variable of the iteration operator can only select array type data.
      if (
        (!isEmpty(types) && types?.some((x) => x === dataType)) ||
        isEmpty(types)
      ) {
        click(option);
      }
    },
    [click, types],
  );

  const handleMenuClick = useCallback(() => {
    if (isEmpty(types) || types?.some((x) => x === JsonSchemaDataType.Object)) {
      click(data);
    }
  }, [click, data, types]);

  const renderAgentStructuredOutput = useCallback(
    (values: any, option: { label: ReactNode; value: string }) => {
      const properties =
        get(values, 'properties') || get(values, 'items.properties');

      if (isPlainObject(values) && properties) {
        return (
          <ul className="border-l">
            {Object.entries(properties).map(([key, value]) => {
              const nextOption = {
                label: option.label + `.${key}`,
                value: option.value + `.${key}`,
              };

              const { dataType, compositeDataType } =
                getStructuredDatatype(value);

              if (
                isEmpty(types) ||
                (!isEmpty(types) &&
                  (types?.some((x) => x === compositeDataType) ||
                    hasSpecificTypeChild(value ?? {}, types)))
              ) {
                return (
                  <li key={key} className="pl-1">
                    <div
                      onClick={handleSubMenuClick(
                        nextOption,
                        compositeDataType,
                      )}
                      className="hover:bg-bg-card p-1 text-text-primary rounded-sm flex justify-between"
                    >
                      {key}
                      <span className="text-text-secondary">
                        {compositeDataType}
                      </span>
                    </div>
                    {[JsonSchemaDataType.Object, JsonSchemaDataType.Array].some(
                      (x) => x === dataType,
                    ) && renderAgentStructuredOutput(value, nextOption)}
                  </li>
                );
              }

              return null;
            })}
          </ul>
        );
      }

      return <div></div>;
    },
    [handleSubMenuClick, types],
  );

  if (
    !isEmpty(types) &&
    !hasSpecificTypeChild(structuredOutput, types) &&
    !types.some((x) => x === JsonSchemaDataType.Object)
  ) {
    return null;
  }

  return (
    <HoverCard key={data.value} openDelay={100} closeDelay={100}>
      <HoverCardTrigger asChild>
        <li
          onClick={handleMenuClick}
          className="hover:bg-bg-card py-1 px-2 text-text-primary rounded-sm text-sm flex justify-between items-center gap-2"
        >
          <div className="flex justify-between flex-1">
            {data.label} <span className="text-text-secondary">object</span>
          </div>
          <ChevronRight className="size-3.5 text-text-secondary" />
        </li>
      </HoverCardTrigger>
      <HoverCardContent
        side="left"
        align="start"
        className={cn(
          'min-w-72  border border-border rounded-md shadow-lg p-0',
        )}
      >
        <section className="p-2">
          <div className="p-1">
            {t('flow.structuredOutput.structuredOutput')}
          </div>
          {renderAgentStructuredOutput(structuredOutput, data)}
        </section>
      </HoverCardContent>
    </HoverCard>
  );
}
