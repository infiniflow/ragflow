import {
  TooltipContent,
  TooltipNode,
  TooltipTrigger,
} from '@/components/xyflow/tooltip-node';
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

function IconWrapper({ children, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className="p-1.5 bg-text-title rounded-sm cursor-pointer" {...props}>
      {children}
    </div>
  );
}

type ToolBarProps = {
  selected?: boolean | undefined;
  label: string;
  id: string;
  showRun?: boolean;
} & PropsWithChildren;

export function ToolBar({
  selected,
  children,
  label,
  id,
  showRun = true,
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
      <TooltipTrigger>{children}</TooltipTrigger>

      <TooltipContent position={Position.Top}>
        <section className="flex gap-2 items-center">
          {showRun && (
            <IconWrapper>
              <Play className="size-3.5" data-play />
            </IconWrapper>
          )}{' '}
          <IconWrapper onClick={handleDuplicate}>
            <Copy className="size-3.5" />
          </IconWrapper>
          <IconWrapper onClick={deleteNode}>
            <Trash2 className="size-3.5" />
          </IconWrapper>
        </section>
      </TooltipContent>
    </TooltipNode>
  );
}
