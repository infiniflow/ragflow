import { cn } from '@/lib/utils';
import { memo } from 'react';
import { Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
import { useTranslate } from '@/hooks/use-translate';
import { RunTooltip } from '@/components/run-tooltip';
import { NextNodePopover } from '@/components/next-node-popover';
import { Play } from 'lucide-react';

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
    <section className="flex justify-end items-center pb-1 gap-2 text-primary">
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

const InnerNodeHeader = ({
  label,
  name,
  className,
  wrapperClassName,
}: IProps) => {
  return (
    <section className={cn(wrapperClassName, 'pb-4')}>
      <div className={cn(className, 'flex gap-2.5')}>
        <OperatorIcon name={label as Operator}></OperatorIcon>
        <span className="truncate text-center font-semibold text-sm">
          {name}
        </span>
      </div>
    </section>
  );
};

const NodeHeader = memo(InnerNodeHeader);

export default NodeHeader;
