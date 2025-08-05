import { useSetModalState } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { Handle, HandleProps } from '@xyflow/react';
import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import { HandleContext } from '../../context';
import { InnerNextStepDropdown } from './dropdown/next-step-dropdown';

export function CommonHandle({
  className,
  nodeId,
  ...props
}: HandleProps & { nodeId: string }) {
  const { visible, hideModal, showModal } = useSetModalState();

  const value = useMemo(
    () => ({
      nodeId,
      id: props.id,
      type: props.type,
      position: props.position,
    }),
    [nodeId, props.id, props.position, props.type],
  );

  return (
    <HandleContext.Provider value={value}>
      <Handle
        {...props}
        className={cn(
          'inline-flex justify-center items-center !bg-background-checked !size-4 !rounded-sm !border-none ',
          className,
        )}
        onClick={(e) => {
          e.stopPropagation();
          showModal();
        }}
      >
        <Plus className="size-3 pointer-events-none text-text-title-invert" />
        {visible && (
          <InnerNextStepDropdown hideModal={hideModal}>
            <span></span>
          </InnerNextStepDropdown>
        )}
      </Handle>
    </HandleContext.Provider>
  );
}
