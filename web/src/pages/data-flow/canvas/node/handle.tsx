import { useSetModalState } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { Handle, HandleProps } from '@xyflow/react';
import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import { HandleContext } from '../../context';
import { useDropdownManager } from '../context';
import { InnerNextStepDropdown } from './dropdown/next-step-dropdown';

export function CommonHandle({
  className,
  nodeId,
  ...props
}: HandleProps & { nodeId: string }) {
  const { visible, hideModal, showModal } = useSetModalState();

  const { canShowDropdown, setActiveDropdown, clearActiveDropdown } =
    useDropdownManager();

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
        className={cn(
          'inline-flex justify-center items-center !bg-accent-primary !size-4 !rounded-sm !border-none ',
          className,
        )}
        onClick={(e) => {
          e.stopPropagation();

          if (!canShowDropdown()) {
            return;
          }

          setActiveDropdown('handle');
          showModal();
        }}
      >
        <Plus className="size-3 pointer-events-none text-text-title-invert" />
        {visible && (
          <InnerNextStepDropdown
            hideModal={() => {
              hideModal();
              clearActiveDropdown();
            }}
          >
            <span></span>
          </InnerNextStepDropdown>
        )}
      </Handle>
    </HandleContext.Provider>
  );
}
