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
import { Operator } from '@/pages/agent/constant';
import { AgentInstanceContext, HandleContext } from '@/pages/agent/context';
import OperatorIcon from '@/pages/agent/operator-icon';
import { Position } from '@xyflow/react';
import { t } from 'i18next';
import { lowerFirst } from 'lodash';
import {
  PropsWithChildren,
  createContext,
  memo,
  useContext,
  useEffect,
  useRef,
} from 'react';
import { useTranslation } from 'react-i18next';

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
  const { t } = useTranslation();

  const handleClick = (operator: Operator) => {
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
      : undefined;

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
        {t(`flow.${lowerFirst(operator)}`)}
      </div>
    );

    return (
      <Tooltip key={operator}>
        <TooltipTrigger asChild>
          {isCustomDropdown ? (
            <li onClick={() => handleClick(operator)}>{commonContent}</li>
          ) : (
            <DropdownMenuItem
              key={operator}
              className="hover:bg-background-card py-1 px-3 cursor-pointer rounded-sm flex gap-2 items-center justify-start"
              onClick={() => handleClick(operator)}
              onSelect={() => hideModal?.()}
            >
              <OperatorIcon name={operator} />
              {t(`flow.${lowerFirst(operator)}`)}
            </DropdownMenuItem>
          )}
        </TooltipTrigger>
        <TooltipContent side="right">
          <p>{t(`flow.${lowerFirst(operator)}Description`)}</p>
        </TooltipContent>
      </Tooltip>
    );
  };

  return <ul className="space-y-2">{operators.map(renderOperatorItem)}</ul>;
}

function AccordionOperators({
  isCustomDropdown = false,
  mousePosition,
}: {
  isCustomDropdown?: boolean;
  mousePosition?: { x: number; y: number };
}) {
  return (
    <Accordion
      type="multiple"
      className="px-2 text-text-title max-h-[45vh] overflow-auto"
      defaultValue={['item-1', 'item-2', 'item-3', 'item-4', 'item-5']}
    >
      <AccordionItem value="item-1">
        <AccordionTrigger className="text-xl">
          {t('flow.foundation')}
        </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Agent, Operator.Retrieval]}
            isCustomDropdown={isCustomDropdown}
            mousePosition={mousePosition}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-2">
        <AccordionTrigger className="text-xl">
          {t('flow.dialog')}
        </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Message, Operator.UserFillUp]}
            isCustomDropdown={isCustomDropdown}
            mousePosition={mousePosition}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-3">
        <AccordionTrigger className="text-xl">
          {t('flow.flow')}
        </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[
              Operator.Switch,
              Operator.Iteration,
              Operator.Categorize,
            ]}
            isCustomDropdown={isCustomDropdown}
            mousePosition={mousePosition}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-4">
        <AccordionTrigger className="text-xl">
          {t('flow.dataManipulation')}
        </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Code, Operator.StringTransform]}
            isCustomDropdown={isCustomDropdown}
            mousePosition={mousePosition}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-5">
        <AccordionTrigger className="text-xl">
          {t('flow.tools')}
        </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[
              Operator.TavilySearch,
              Operator.TavilyExtract,
              Operator.ExeSQL,
              Operator.Google,
              Operator.YahooFinance,
              Operator.Email,
              Operator.DuckDuckGo,
              Operator.Wikipedia,
              Operator.GoogleScholar,
              Operator.ArXiv,
              Operator.PubMed,
              Operator.GitHub,
              Operator.Invoke,
              Operator.WenCai,
            ]}
            isCustomDropdown={isCustomDropdown}
            mousePosition={mousePosition}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
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
