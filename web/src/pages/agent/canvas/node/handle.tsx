import { useSetModalState } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { Handle, HandleProps, Position } from '@xyflow/react';
import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import { NodeHandleId } from '../../constant';
import { HandleContext } from '../../context';
import { useIsPipeline } from '../../hooks/use-is-pipeline';
import useGraphStore from '../../store';
import { useDropdownManager } from '../context';
import { NextStepDropdown } from './dropdown/next-step-dropdown';

export function CommonHandle({
  className,
  nodeId,
  ...props
}: HandleProps & { nodeId: string }) {
  const { visible, hideModal, showModal } = useSetModalState();
  const { canShowDropdown, setActiveDropdown, clearActiveDropdown } =
    useDropdownManager();
  const { hasChildNode } = useGraphStore((state) => state);
  const isPipeline = useIsPipeline();

  const isConnectable = !(isPipeline && hasChildNode(nodeId)); // Using useMemo will cause isConnectable to not be updated when the subsequent connection line is deleted

  const value = useMemo(
    () => ({
      nodeId,
      id: props.id || undefined,
      type: props.type,
      position: props.position,
      isFromConnectionDrag: false,
    }),
    [nodeId, props.id, props.position, props.type],
  );

  return (
    <HandleContext.Provider value={value}>
      <Handle
        {...props}
        isConnectable={isConnectable}
        className={cn(
          'inline-flex justify-center items-center !bg-accent-primary !border-none group-hover:!size-4 group-hover:!rounded-sm',
          className,
        )}
        onClick={(e) => {
          e.stopPropagation();

          if (!isConnectable) {
            return;
          }

          if (!canShowDropdown()) {
            return;
          }

          setActiveDropdown('handle');
          showModal();
        }}
      >
        <Plus className="size-3 pointer-events-none text-white hidden group-hover:inline-block" />
        {visible && (
          <NextStepDropdown
            nodeId={nodeId}
            hideModal={() => {
              hideModal();
              clearActiveDropdown();
            }}
          >
            <span></span>
          </NextStepDropdown>
        )}
      </Handle>
    </HandleContext.Provider>
  );
}

export function LeftEndHandle({
  isConnectable,
  ...props
}: Omit<HandleProps, 'type' | 'position'>) {
  return (
    <Handle
      isConnectable={isConnectable}
      className="!bg-accent-primary !size-2"
      id={NodeHandleId.End}
      type="target"
      position={Position.Left}
      {...props}
    ></Handle>
  );
}
