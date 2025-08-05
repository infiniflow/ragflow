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
import { lowerFirst } from 'lodash';
import { PropsWithChildren, createContext, memo, useContext } from 'react';
import { useTranslation } from 'react-i18next';

type OperatorItemProps = { operators: Operator[] };

const HideModalContext = createContext<IModalProps<any>['showModal']>(() => {});

function OperatorItemList({ operators }: OperatorItemProps) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const { nodeId, id, position } = useContext(HandleContext);
  const hideModal = useContext(HideModalContext);
  const { t } = useTranslation();

  return (
    <ul className="space-y-2">
      {operators.map((x) => {
        return (
          <Tooltip key={x}>
            <TooltipTrigger asChild>
              <DropdownMenuItem
                key={x}
                className="hover:bg-background-card py-1 px-3 cursor-pointer rounded-sm flex gap-2 items-center justify-start"
                onClick={addCanvasNode(x, {
                  nodeId,
                  id,
                  position,
                })}
                onSelect={() => hideModal?.()}
              >
                <OperatorIcon name={x}></OperatorIcon>
                {t(`flow.${lowerFirst(x)}`)}
              </DropdownMenuItem>
            </TooltipTrigger>
            <TooltipContent side="right">
              <p>{t(`flow.${lowerFirst(x)}Description`)}</p>
            </TooltipContent>
          </Tooltip>
        );
      })}
    </ul>
  );
}

function AccordionOperators() {
  return (
    <Accordion
      type="multiple"
      className="px-2 text-text-title max-h-[45vh] overflow-auto"
      defaultValue={['item-1', 'item-2', 'item-3', 'item-4', 'item-5']}
    >
      <AccordionItem value="item-1">
        <AccordionTrigger className="text-xl">Foundation</AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Agent, Operator.Retrieval]}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-2">
        <AccordionTrigger className="text-xl">Dialogue </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Message, Operator.UserFillUp]}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-3">
        <AccordionTrigger className="text-xl">Flow</AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[
              Operator.Switch,
              Operator.Iteration,
              Operator.Categorize,
            ]}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-4">
        <AccordionTrigger className="text-xl">
          Data Manipulation
        </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Code, Operator.StringTransform]}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-5">
        <AccordionTrigger className="text-xl">Tools</AccordionTrigger>
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
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}

export function InnerNextStepDropdown({
  children,
  hideModal,
}: PropsWithChildren & IModalProps<any>) {
  return (
    <DropdownMenu open onOpenChange={hideModal}>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent
        onClick={(e) => e.stopPropagation()}
        className="w-[300px] font-semibold"
      >
        <DropdownMenuLabel>Next Step</DropdownMenuLabel>
        <HideModalContext.Provider value={hideModal}>
          <AccordionOperators></AccordionOperators>
        </HideModalContext.Provider>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export const NextStepDropdown = memo(InnerNextStepDropdown);
