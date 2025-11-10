import { DropdownMenuItem } from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Operator } from '@/constants/agent';
import { IModalProps } from '@/interfaces/common';
import { AgentInstanceContext, HandleContext } from '@/pages/agent/context';
import OperatorIcon from '@/pages/agent/operator-icon';
import { Position } from '@xyflow/react';
import { lowerFirst } from 'lodash';
import { createContext, useContext } from 'react';
import { useTranslation } from 'react-i18next';

export type OperatorItemProps = {
  operators: Operator[];
  isCustomDropdown?: boolean;
  mousePosition?: { x: number; y: number };
};

export const HideModalContext = createContext<IModalProps<any>['showModal']>(
  () => {},
);
export const OnNodeCreatedContext = createContext<
  ((newNodeId: string) => void) | undefined
>(undefined);

export function OperatorItemList({
  operators,
  isCustomDropdown = false,
  mousePosition,
}: OperatorItemProps) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const handleContext = useContext(HandleContext);
  const hideModal = useContext(HideModalContext);
  const onNodeCreated = useContext(OnNodeCreatedContext);
  const { t } = useTranslation();

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
        {t(`flow.${lowerFirst(operator)}`)}
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
              {t(`flow.${lowerFirst(operator)}`)}
            </DropdownMenuItem>
          )}
        </TooltipTrigger>
        <TooltipContent side="right" sideOffset={24}>
          <p>{t(`flow.${lowerFirst(operator)}Description`)}</p>
        </TooltipContent>
      </Tooltip>
    );
  };

  return (
    <ul className="space-y-2 text-text-primary font-normal">
      {operators.map(renderOperatorItem)}
    </ul>
  );
}
