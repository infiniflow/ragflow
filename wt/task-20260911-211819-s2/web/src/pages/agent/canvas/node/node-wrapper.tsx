import { cn } from '@/lib/utils';
import { Loader } from 'lucide-react';
import { HTMLAttributes, useContext } from 'react';
import { AgentInstanceContext } from '../../context';

type IProps = HTMLAttributes<HTMLDivElement> & { selected?: boolean };

export function NodeWrapper({ children, className, selected, id }: IProps) {
  const { currentSendLoading, startButNotFinishedNodeIds = [] } =
    useContext(AgentInstanceContext);
  return (
    <section
      className={cn(
        'bg-bg-component p-2.5 rounded-md w-[200px] border border-border-button text-xs group hover:shadow-md',
        { 'border border-accent-primary': selected },
        className,
      )}
    >
      {id &&
        startButNotFinishedNodeIds.indexOf(id as string) > -1 &&
        currentSendLoading && (
          <div className=" absolute right-0 left-0 top-0 flex items-start justify-end p-2">
            <Loader size={12} className=" animate-spin" />
          </div>
        )}
      {children}
    </section>
  );
}
