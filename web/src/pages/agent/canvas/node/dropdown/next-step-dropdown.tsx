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
import { Operator } from '@/pages/agent/constant';
import { AgentInstanceContext, HandleContext } from '@/pages/agent/context';
import OperatorIcon from '@/pages/agent/operator-icon';
import { PropsWithChildren, useContext } from 'react';

type OperatorItemProps = { operators: Operator[] };

function OperatorItemList({ operators }: OperatorItemProps) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const { nodeId, id, type, position } = useContext(HandleContext);

  return (
    <ul className="space-y-2">
      {operators.map((x) => {
        return (
          <DropdownMenuItem
            key={x}
            className="hover:bg-background-card py-1 px-3 cursor-pointer rounded-sm flex gap-2 items-center justify-start"
            onClick={addCanvasNode(x, {
              nodeId,
              id,
              position,
            })}
          >
            <OperatorIcon name={x}></OperatorIcon>
            {x}
          </DropdownMenuItem>
        );
      })}
    </ul>
  );
}

function AccordionOperators() {
  return (
    <Accordion
      type="multiple"
      className="px-2 text-text-title"
      defaultValue={['item-1', 'item-2', 'item-3', 'item-4', 'item-5']}
    >
      <AccordionItem value="item-1">
        <AccordionTrigger className="text-xl">AI</AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList
            operators={[Operator.Agent, Operator.Retrieval]}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-2">
        <AccordionTrigger className="text-xl">Dialogue </AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList operators={[Operator.Message]}></OperatorItemList>
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
          <OperatorItemList operators={[Operator.Code]}></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
      <AccordionItem value="item-5">
        <AccordionTrigger className="text-xl">Tools</AccordionTrigger>
        <AccordionContent className="flex flex-col gap-4 text-balance">
          <OperatorItemList operators={[]}></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}

export function NextStepDropdown({ children }: PropsWithChildren) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent
        onClick={(e) => e.stopPropagation()}
        className="w-[300px] font-semibold"
      >
        <DropdownMenuLabel>Next Step</DropdownMenuLabel>
        <AccordionOperators></AccordionOperators>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
