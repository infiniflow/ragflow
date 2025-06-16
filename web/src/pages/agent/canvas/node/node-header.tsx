import { cn } from '@/lib/utils';
import { memo } from 'react';
import { Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
interface IProps {
  id: string;
  label: string;
  name: string;
  gap?: number;
  className?: string;
  wrapperClassName?: string;
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
