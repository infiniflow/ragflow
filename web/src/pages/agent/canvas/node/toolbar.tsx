import {
  TooltipContent,
  TooltipNode,
  TooltipTrigger,
} from '@/components/xyflow/tooltip-node';
import { cn } from '@/lib/utils';
import { Position } from '@xyflow/react';
import { Copy, Play, Trash2 } from 'lucide-react';
import {
  HTMLAttributes,
  MouseEventHandler,
  PropsWithChildren,
  useCallback,
} from 'react';
import { Operator } from '../../constant';
import { useDuplicateNode } from '../../hooks';
import useGraphStore from '../../store';

function IconWrapper({
  children,
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'p-1.5 bg-bg-component border border-border-button rounded-sm cursor-pointer hover:text-text-primary',
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

type ToolBarProps = {
  selected?: boolean | undefined;
  label: string;
  id: string;
  showRun?: boolean;
  showCopy?: boolean;
} & PropsWithChildren;

export function ToolBar({
  selected,
  children,
  label,
  id,
  showRun = true,
  showCopy = true,
}: ToolBarProps) {
  const deleteNodeById = useGraphStore((store) => store.deleteNodeById);
  const deleteIterationNodeById = useGraphStore(
    (store) => store.deleteIterationNodeById,
  );

  const deleteNode: MouseEventHandler<HTMLDivElement> = useCallback(
    (e) => {
      e.stopPropagation();
      if (label === Operator.Iteration) {
        deleteIterationNodeById(id);
      } else {
        deleteNodeById(id);
      }
    },
    [deleteIterationNodeById, deleteNodeById, id, label],
  );

  const duplicateNode = useDuplicateNode();

  const handleDuplicate: MouseEventHandler<HTMLDivElement> = useCallback(
    (e) => {
      e.stopPropagation();
      duplicateNode(id, label);
    },
    [duplicateNode, id, label],
  );

  return (
    <TooltipNode selected={selected}>
      <TooltipTrigger className="h-full">{children}</TooltipTrigger>

      <TooltipContent position={Position.Top}>
        <section className="flex gap-2 items-center text-text-secondary">
          {showRun && (
            <IconWrapper>
              <Play className="size-3.5" data-play />
            </IconWrapper>
          )}
          {showCopy && (
            <IconWrapper onClick={handleDuplicate}>
              <Copy className="size-3.5" />
            </IconWrapper>
          )}
          <IconWrapper
            onClick={deleteNode}
            className="hover:text-state-error hover:border-state-error"
          >
            <Trash2 className="size-3.5" />
          </IconWrapper>
        </section>
      </TooltipContent>
    </TooltipNode>
  );
}
