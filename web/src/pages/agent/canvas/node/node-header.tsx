import { useTranslate } from '@/hooks/common-hooks';
import { Flex } from 'antd';
import { Play } from 'lucide-react';
import { Operator, operatorMap } from '../../constant';
import OperatorIcon from '../../operator-icon';
import { needsSingleStepDebugging } from '../../utils';
import NodeDropdown from './dropdown';
import { NextNodePopover } from './popover';

import { RunTooltip } from '../../flow-tooltip';
interface IProps {
  id: string;
  label: string;
  name: string;
  gap?: number;
  className?: string;
  wrapperClassName?: string;
}

const ExcludedRunStateOperators = [Operator.Answer];

export function RunStatus({ id, name, label }: IProps) {
  const { t } = useTranslate('flow');
  return (
    <section className="flex  justify-end items-center pb-1 gap-2 text-blue-600">
      {needsSingleStepDebugging(label) && (
        <RunTooltip>
          <Play className="size-3 cursor-pointer" data-play />
        </RunTooltip> // data-play is used to trigger single step debugging
      )}
      <NextNodePopover nodeId={id} name={name}>
        <span className="cursor-pointer text-[10px]">
          {t('operationResults')}
        </span>
      </NextNodePopover>
    </section>
  );
}

const NodeHeader = ({
  label,
  id,
  name,
  gap = 4,
  className,
  wrapperClassName,
}: IProps) => {
  return (
    <section className={wrapperClassName}>
      {!ExcludedRunStateOperators.includes(label as Operator) && (
        <RunStatus id={id} name={name} label={label}></RunStatus>
      )}
      <Flex
        flex={1}
        align="center"
        justify={'space-between'}
        gap={gap}
        className={className}
      >
        <OperatorIcon
          name={label as Operator}
          color={operatorMap[label as Operator]?.color}
        ></OperatorIcon>
        <span className="truncate text-center font-semibold text-sm">
          {name}
        </span>
        <NodeDropdown id={id} label={label}></NodeDropdown>
      </Flex>
    </section>
  );
};

export default NodeHeader;
