import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { IModalProps } from '@/interfaces/common';
import { useGetNodeDescription, useGetNodeName } from '@/pages/data-flow/hooks';
import useGraphStore from '@/pages/data-flow/store';
import { Position } from '@xyflow/react';
import { t } from 'i18next';
import {
  PropsWithChildren,
  createContext,
  memo,
  useContext,
  useEffect,
  useMemo,
  useRef,
} from 'react';
import { Operator, SingleOperators } from '../../../constant';
import { AgentInstanceContext, HandleContext } from '../../../context';
import OperatorIcon from '../../../operator-icon';

type OperatorItemProps = {
  operators: Operator[];
  isCustomDropdown?: boolean;
  mousePosition?: { x: number; y: number };
};

const HideModalContext = createContext<IModalProps<any>['showModal']>(() => {});
const OnNodeCreatedContext = createContext<
  ((newNodeId: string) => void) | undefined
>(undefined);

function OperatorItemList({
  operators,
  isCustomDropdown = false,
  mousePosition,
}: OperatorItemProps) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const handleContext = useContext(HandleContext);
  const hideModal = useContext(HideModalContext);
  const onNodeCreated = useContext(OnNodeCreatedContext);

  const getNodeName = useGetNodeName();
  const getNodeDescription = useGetNodeDescription();

  const handleClick =
    (operator: Operator): React.MouseEventHandler<HTMLElement> =>
    (e) => {
      const contextData = handleContext || {
        nodeId: '',
        id: '',
        type: 'source' as const,
        position: Position.Right,
        isFromConnectionDrag: true,
      };

      const mockEvent = mousePosition
        ? {
            clientX: mousePosition.x,
            clientY: mousePosition.y,
          }
        : e;

      const newNodeId = addCanvasNode(operator, contextData)(mockEvent);

      if (onNodeCreated && newNodeId) {
        onNodeCreated(newNodeId);
      }

      hideModal?.();
    };

  const renderOperatorItem = (operator: Operator) => {
    const commonContent = (
      <div className="hover:bg-background-card py-1 px-3 cursor-pointer rounded-sm flex gap-2 items-center justify-start">
        <OperatorIcon name={operator} />
        {getNodeName(operator)}
      </div>
    );

    return (
      <Tooltip key={operator}>
        <TooltipTrigger asChild>
          {isCustomDropdown ? (
            <li onClick={handleClick(operator)}>{commonContent}</li>
          ) : (
            <DropdownMenuItem
              key={operator}
              className="hover:bg-background-card py-1 px-3 cursor-pointer rounded-sm flex gap-2 items-center justify-start"
              onClick={handleClick(operator)}
              onSelect={() => hideModal?.()}
            >
              <OperatorIcon name={operator} />
              {getNodeName(operator)}
            </DropdownMenuItem>
          )}
        </TooltipTrigger>
        <TooltipContent side="right">
          <p>{getNodeDescription(operator)}</p>
        </TooltipContent>
      </Tooltip>
    );
  };

  return <ul className="space-y-2">{operators.map(renderOperatorItem)}</ul>;
}

// Limit the number of operators of a certain type on the canvas to only one
function useRestrictSingleOperatorOnCanvas() {
  const list: Operator[] = [];
  const { findNodeByName } = useGraphStore((state) => state);

  SingleOperators.forEach((operator) => {
    if (!findNodeByName(operator)) {
      list.push(operator);
    }
  });

  return list;
}

function AccordionOperators({
  isCustomDropdown = false,
  mousePosition,
}: {
  isCustomDropdown?: boolean;
  mousePosition?: { x: number; y: number };
}) {
  const singleOperators = useRestrictSingleOperatorOnCanvas();
  const operators = useMemo(() => {
    const list = [...singleOperators];
    list.push(Operator.Extractor);
    return list;
  }, [singleOperators]);

  return (
    <OperatorItemList
      operators={operators}
      isCustomDropdown={isCustomDropdown}
      mousePosition={mousePosition}
    ></OperatorItemList>
  );
}

export function InnerNextStepDropdown({
  children,
  hideModal,
  position,
  onNodeCreated,
}: PropsWithChildren &
  IModalProps<any> & {
    position?: { x: number; y: number };
    onNodeCreated?: (newNodeId: string) => void;
  }) {
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (position && hideModal) {
      const handleKeyDown = (event: KeyboardEvent) => {
        if (event.key === 'Escape') {
          hideModal();
        }
      };

      document.addEventListener('keydown', handleKeyDown);

      return () => {
        document.removeEventListener('keydown', handleKeyDown);
      };
    }
  }, [position, hideModal]);

  if (position) {
    return (
      <div
        ref={dropdownRef}
        style={{
          position: 'fixed',
          left: position.x,
          top: position.y + 10,
          zIndex: 1000,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="w-[300px] font-semibold bg-bg-base border border-border rounded-md shadow-lg">
          <div className="px-3 py-2 border-b border-border">
            <div className="text-sm font-medium">{t('flow.nextStep')}</div>
          </div>
          <HideModalContext.Provider value={hideModal}>
            <OnNodeCreatedContext.Provider value={onNodeCreated}>
              <AccordionOperators
                isCustomDropdown={true}
                mousePosition={position}
              ></AccordionOperators>
            </OnNodeCreatedContext.Provider>
          </HideModalContext.Provider>
        </div>
      </div>
    );
  }

  return (
    <DropdownMenu
      open={true}
      onOpenChange={(open) => {
        if (!open && hideModal) {
          hideModal();
        }
      }}
    >
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent
        onClick={(e) => e.stopPropagation()}
        className="w-[300px] font-semibold"
      >
        <DropdownMenuLabel>{t('flow.nextStep')}</DropdownMenuLabel>
        <HideModalContext.Provider value={hideModal}>
          <AccordionOperators></AccordionOperators>
        </HideModalContext.Provider>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export const NextStepDropdown = memo(InnerNextStepDropdown);
