import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
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
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
} from 'react';
import { Operator } from '../../../constant';
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
  const { findNodeByName } = useGraphStore((state) => state);

  const restrictSingleOperatorOnCanvas = useCallback(
    (singleOperators: Operator[]) => {
      const list: Operator[] = [];
      singleOperators.forEach((operator) => {
        if (!findNodeByName(operator)) {
          list.push(operator);
        }
      });
      return list;
    },
    [findNodeByName],
  );

  return restrictSingleOperatorOnCanvas;
}

function AccordionOperators({
  isCustomDropdown = false,
  mousePosition,
  nodeId,
}: {
  isCustomDropdown?: boolean;
  mousePosition?: { x: number; y: number };
  nodeId?: string;
}) {
  const restrictSingleOperatorOnCanvas = useRestrictSingleOperatorOnCanvas();
  const { getOperatorTypeFromId } = useGraphStore((state) => state);

  const operators = useMemo(() => {
    let list = [
      ...restrictSingleOperatorOnCanvas([Operator.Parser, Operator.Tokenizer]),
    ];
    list.push(Operator.Extractor);
    return list;
  }, [restrictSingleOperatorOnCanvas]);

  const chunkerOperators = useMemo(() => {
    return [
      ...restrictSingleOperatorOnCanvas([
        Operator.Splitter,
        Operator.HierarchicalMerger,
      ]),
    ];
  }, [restrictSingleOperatorOnCanvas]);

  const showChunker = useMemo(() => {
    return (
      getOperatorTypeFromId(nodeId) !== Operator.Extractor &&
      chunkerOperators.length > 0
    );
  }, [chunkerOperators.length, getOperatorTypeFromId, nodeId]);

  return (
    <>
      <OperatorItemList
        operators={operators}
        isCustomDropdown={isCustomDropdown}
        mousePosition={mousePosition}
      ></OperatorItemList>
      {showChunker && (
        <Accordion
          type="single"
          collapsible
          className="w-full px-4"
          defaultValue="item-1"
        >
          <AccordionItem value="item-1">
            <AccordionTrigger>Chunker</AccordionTrigger>
            <AccordionContent className="flex flex-col gap-4 text-balance">
              <OperatorItemList
                operators={chunkerOperators}
                isCustomDropdown={isCustomDropdown}
                mousePosition={mousePosition}
              ></OperatorItemList>
            </AccordionContent>
          </AccordionItem>
        </Accordion>
      )}
    </>
  );
}

type NextStepDropdownProps = PropsWithChildren &
  IModalProps<any> & {
    position?: { x: number; y: number };
    onNodeCreated?: (newNodeId: string) => void;
    nodeId?: string;
  };
export function InnerNextStepDropdown({
  children,
  hideModal,
  position,
  onNodeCreated,
  nodeId,
}: NextStepDropdownProps) {
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
                nodeId={nodeId}
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
          <AccordionOperators nodeId={nodeId}></AccordionOperators>
        </HideModalContext.Provider>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export const NextStepDropdown = memo(InnerNextStepDropdown);
