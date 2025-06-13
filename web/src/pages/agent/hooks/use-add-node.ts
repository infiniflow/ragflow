import { useFetchModelId } from '@/hooks/logic-hooks';
import { Node, Position, ReactFlowInstance } from '@xyflow/react';
import humanId from 'human-id';
import { lowerFirst } from 'lodash';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  NodeMap,
  Operator,
  initialAgentValues,
  initialAkShareValues,
  initialArXivValues,
  initialBaiduFanyiValues,
  initialBaiduValues,
  initialBeginValues,
  initialBingValues,
  initialCategorizeValues,
  initialCodeValues,
  initialConcentratorValues,
  initialCrawlerValues,
  initialDeepLValues,
  initialDuckValues,
  initialEmailValues,
  initialExeSqlValues,
  initialGenerateValues,
  initialGithubValues,
  initialGoogleScholarValues,
  initialGoogleValues,
  initialInvokeValues,
  initialIterationValues,
  initialJin10Values,
  initialKeywordExtractValues,
  initialMessageValues,
  initialNoteValues,
  initialPubMedValues,
  initialQWeatherValues,
  initialRelevantValues,
  initialRetrievalValues,
  initialRewriteQuestionValues,
  initialSwitchValues,
  initialTemplateValues,
  initialTuShareValues,
  initialWaitingDialogueValues,
  initialWenCaiValues,
  initialWikipediaValues,
  initialYahooFinanceValues,
} from '../constant';
import useGraphStore from '../store';
import {
  generateNodeNamesWithIncreasingIndex,
  getNodeDragHandle,
  getRelativePositionToIterationNode,
} from '../utils';

export const useInitializeOperatorParams = () => {
  const llmId = useFetchModelId();

  const initialFormValuesMap = useMemo(() => {
    return {
      [Operator.Begin]: initialBeginValues,
      [Operator.Retrieval]: initialRetrievalValues,
      [Operator.Generate]: { ...initialGenerateValues, llm_id: llmId },
      [Operator.Answer]: {},
      [Operator.Categorize]: { ...initialCategorizeValues, llm_id: llmId },
      [Operator.Relevant]: { ...initialRelevantValues, llm_id: llmId },
      [Operator.RewriteQuestion]: {
        ...initialRewriteQuestionValues,
        llm_id: llmId,
      },
      [Operator.Message]: initialMessageValues,
      [Operator.KeywordExtract]: {
        ...initialKeywordExtractValues,
        llm_id: llmId,
      },
      [Operator.DuckDuckGo]: initialDuckValues,
      [Operator.Baidu]: initialBaiduValues,
      [Operator.Wikipedia]: initialWikipediaValues,
      [Operator.PubMed]: initialPubMedValues,
      [Operator.ArXiv]: initialArXivValues,
      [Operator.Google]: initialGoogleValues,
      [Operator.Bing]: initialBingValues,
      [Operator.GoogleScholar]: initialGoogleScholarValues,
      [Operator.DeepL]: initialDeepLValues,
      [Operator.GitHub]: initialGithubValues,
      [Operator.BaiduFanyi]: initialBaiduFanyiValues,
      [Operator.QWeather]: initialQWeatherValues,
      [Operator.ExeSQL]: { ...initialExeSqlValues, llm_id: llmId },
      [Operator.Switch]: initialSwitchValues,
      [Operator.WenCai]: initialWenCaiValues,
      [Operator.AkShare]: initialAkShareValues,
      [Operator.YahooFinance]: initialYahooFinanceValues,
      [Operator.Jin10]: initialJin10Values,
      [Operator.Concentrator]: initialConcentratorValues,
      [Operator.TuShare]: initialTuShareValues,
      [Operator.Note]: initialNoteValues,
      [Operator.Crawler]: initialCrawlerValues,
      [Operator.Invoke]: initialInvokeValues,
      [Operator.Template]: initialTemplateValues,
      [Operator.Email]: initialEmailValues,
      [Operator.Iteration]: initialIterationValues,
      [Operator.IterationStart]: initialIterationValues,
      [Operator.Code]: initialCodeValues,
      [Operator.WaitingDialogue]: initialWaitingDialogueValues,
      [Operator.Agent]: { ...initialAgentValues, llm_id: llmId },
    };
  }, [llmId]);

  const initializeOperatorParams = useCallback(
    (operatorName: Operator) => {
      return initialFormValuesMap[operatorName];
    },
    [initialFormValuesMap],
  );

  return initializeOperatorParams;
};

export const useGetNodeName = () => {
  const { t } = useTranslation();

  return (type: string) => {
    const name = t(`flow.${lowerFirst(type)}`);
    return name;
  };
};

export function useAddNode(reactFlowInstance?: ReactFlowInstance<any, any>) {
  const addNode = useGraphStore((state) => state.addNode);
  const getNode = useGraphStore((state) => state.getNode);
  const addEdge = useGraphStore((state) => state.addEdge);
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  const getNodeName = useGetNodeName();
  const initializeOperatorParams = useInitializeOperatorParams();
  //   const [reactFlowInstance, setReactFlowInstance] =
  //     useState<ReactFlowInstance<any, any>>();

  const addCanvasNode = useCallback(
    (type: string, id?: string) => (event: React.MouseEvent<HTMLElement>) => {
      // reactFlowInstance.project was renamed to reactFlowInstance.screenToFlowPosition
      // and you don't need to subtract the reactFlowBounds.left/top anymore
      // details: https://@xyflow/react.dev/whats-new/2023-11-10
      const position = reactFlowInstance?.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });

      const newNode: Node<any> = {
        id: `${type}:${humanId()}`,
        type: NodeMap[type as Operator] || 'ragNode',
        position: position || {
          x: 0,
          y: 0,
        },
        data: {
          label: `${type}`,
          name: generateNodeNamesWithIncreasingIndex(getNodeName(type), nodes),
          form: initializeOperatorParams(type as Operator),
        },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        dragHandle: getNodeDragHandle(type),
      };

      if (type === Operator.Iteration) {
        newNode.width = 500;
        newNode.height = 250;
        const iterationStartNode: Node<any> = {
          id: `${Operator.IterationStart}:${humanId()}`,
          type: 'iterationStartNode',
          position: { x: 50, y: 100 },
          // draggable: false,
          data: {
            label: Operator.IterationStart,
            name: Operator.IterationStart,
            form: {},
          },
          parentId: newNode.id,
          extent: 'parent',
        };
        addNode(newNode);
        addNode(iterationStartNode);
      } else if (type === Operator.Agent) {
        const agentNode = getNode(id);
        if (agentNode) {
          // Calculate the coordinates of child nodes to prevent newly added child nodes from covering other child nodes
          const allChildAgentNodeIds = edges
            .filter((x) => x.source === id && x.sourceHandle === 'e')
            .map((x) => x.target);

          const xAxises = nodes
            .filter((x) => allChildAgentNodeIds.some((y) => y === x.id))
            .map((x) => x.position.x);

          const maxX = Math.max(...xAxises);

          newNode.position = {
            x: xAxises.length > 0 ? maxX + 262 : agentNode.position.x + 82,
            y: agentNode.position.y + 140,
          };
        }
        addNode(newNode);
        if (id) {
          addEdge({
            source: id,
            target: newNode.id,
            sourceHandle: 'e',
            targetHandle: 'f',
          });
        }
      } else {
        const subNodeOfIteration = getRelativePositionToIterationNode(
          nodes,
          position,
        );
        if (subNodeOfIteration) {
          newNode.parentId = subNodeOfIteration.parentId;
          newNode.position = subNodeOfIteration.position;
          newNode.extent = 'parent';
        }
        addNode(newNode);
      }
    },
    [
      addEdge,
      addNode,
      edges,
      getNode,
      getNodeName,
      initializeOperatorParams,
      nodes,
      reactFlowInstance,
    ],
  );

  return { addCanvasNode };
}
