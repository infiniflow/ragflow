import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { getStructuredDatatype } from '@/utils/canvas-util';
import { get, isEmpty, isPlainObject } from 'lodash';
import { ChevronRight } from 'lucide-react';
import {
  ForwardedRef,
  forwardRef,
  PropsWithChildren,
  ReactNode,
  useCallback,
} from 'react';
import { useTranslation } from 'react-i18next';
import { JsonSchemaDataType } from '../../constant';
import { useGetStructuredOutputByValue } from '../../hooks/use-build-structured-output';
import { hasSpecificTypeChild } from '../../utils/filter-agent-structured-output';

type DataItem = { label: ReactNode; value: string; parentLabel?: ReactNode };

type StructuredOutputSecondaryMenuProps = {
  className?: string;
  data: DataItem;
  click(option: { label: ReactNode; value: string }): void;
  types?: JsonSchemaDataType[];
} & PropsWithChildren;

export const StructuredOutputSecondaryMenu = forwardRef(
  function StructuredOutputSecondaryMenu(
    { className, data, click, types = [] }: StructuredOutputSecondaryMenuProps,
    ref: ForwardedRef<HTMLLIElement>,
  ) {
    const { t } = useTranslation();
    const filterStructuredOutput = useGetStructuredOutputByValue();
    const structuredOutput = filterStructuredOutput(data.value);

    const handleSubMenuClick = useCallback(
      (option: { label: ReactNode; value: string }, dataType?: string) =>
        () => {
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
      if (
        isEmpty(types) ||
        types?.some((x) => x === JsonSchemaDataType.Object)
      ) {
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
                        <span className="text-text-secondary text-xs">
                          {compositeDataType}
                        </span>
                      </div>
                      {[
                        JsonSchemaDataType.Object,
                        JsonSchemaDataType.Array,
                      ].some((x) => x === dataType) &&
                        renderAgentStructuredOutput(value, nextOption)}
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
            ref={ref}
            onClick={handleMenuClick}
            className={cn(
              'py-1 px-2 text-text-primary rounded-sm text-sm flex items-center gap-2',
              'hover:bg-bg-card focus-visible:bg-bg-card',
              className,
            )}
          >
            <span>{data.label}</span>
            <span className="ml-auto text-xs text-text-secondary">object</span>
            <ChevronRight className="size-[1em] text-text-secondary" />
          </li>
        </HoverCardTrigger>

        <HoverCardContent
          side="left"
          align="start"
          className="min-w-72 bg-bg-base border-0.5 border-border rounded-md shadow-lg p-0 overflow-hidden"
        >
          <ScrollArea className="p-2">
            <section>
              <header className="mb-2 text-text-secondary">
                {t('flow.structuredOutput.structuredOutput')}
              </header>

              {renderAgentStructuredOutput(structuredOutput, data)}
            </section>
          </ScrollArea>
        </HoverCardContent>
      </HoverCard>
    );
  },
);
