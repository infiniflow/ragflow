import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
import { Operator } from '@/constants/agent';
import useGraphStore from '@/pages/agent/store';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { OperatorItemList } from './operator-item-list';

export function AccordionOperators({
  isCustomDropdown = false,
  mousePosition,
}: {
  isCustomDropdown?: boolean;
  mousePosition?: { x: number; y: number };
}) {
  const { t } = useTranslation();

  return (
    <Accordion
      type="multiple"
      className="px-2 text-text-title max-h-[45vh] overflow-auto scrollbar-none"
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
              Operator.SearXNG,
            ]}
            isCustomDropdown={isCustomDropdown}
            mousePosition={mousePosition}
          ></OperatorItemList>
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
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

export function PipelineAccordionOperators({
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
