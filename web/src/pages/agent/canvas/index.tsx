import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import {
  Background,
  ConnectionMode,
  ControlButton,
  Controls,
  NodeTypes,
  ReactFlow,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { NotebookPen } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { ChatSheet } from '../chat/chat-sheet';
import {
  AgentChatContext,
  AgentChatLogContext,
  AgentInstanceContext,
} from '../context';
import FormSheet from '../form-sheet/next';
import {
  useHandleDrop,
  useSelectCanvasData,
  useValidateConnection,
} from '../hooks';
import { useAddNode } from '../hooks/use-add-node';
import { useBeforeDelete } from '../hooks/use-before-delete';
import { useCacheChatLog } from '../hooks/use-cache-chat-log';
import { useMoveNote } from '../hooks/use-move-note';
import { useShowDrawer, useShowLogSheet } from '../hooks/use-show-drawer';
import { LogSheet } from '../log-sheet';
import RunSheet from '../run-sheet';
import { ButtonEdge } from './edge';
import styles from './index.less';
import { RagNode } from './node';
import { AgentNode } from './node/agent-node';
import { BeginNode } from './node/begin-node';
import { CategorizeNode } from './node/categorize-node';
import { EmailNode } from './node/email-node';
import { GenerateNode } from './node/generate-node';
import { InvokeNode } from './node/invoke-node';
import { IterationNode, IterationStartNode } from './node/iteration-node';
import { KeywordNode } from './node/keyword-node';
import { LogicNode } from './node/logic-node';
import { MessageNode } from './node/message-node';
import NoteNode from './node/note-node';
import { RelevantNode } from './node/relevant-node';
import { RetrievalNode } from './node/retrieval-node';
import { RewriteNode } from './node/rewrite-node';
import { SwitchNode } from './node/switch-node';
import { TemplateNode } from './node/template-node';
import { ToolNode } from './node/tool-node';

const nodeTypes: NodeTypes = {
  ragNode: RagNode,
  categorizeNode: CategorizeNode,
  beginNode: BeginNode,
  relevantNode: RelevantNode,
  logicNode: LogicNode,
  noteNode: NoteNode,
  switchNode: SwitchNode,
  generateNode: GenerateNode,
  retrievalNode: RetrievalNode,
  messageNode: MessageNode,
  rewriteNode: RewriteNode,
  keywordNode: KeywordNode,
  invokeNode: InvokeNode,
  templateNode: TemplateNode,
  emailNode: EmailNode,
  group: IterationNode,
  iterationStartNode: IterationStartNode,
  agentNode: AgentNode,
  toolNode: ToolNode,
};

const edgeTypes = {
  buttonEdge: ButtonEdge,
};

interface IProps {
  drawerVisible: boolean;
  hideDrawer(): void;
}

function AgentCanvas({ drawerVisible, hideDrawer }: IProps) {
  const { t } = useTranslation();
  const {
    nodes,
    edges,
    onConnect,
    onEdgesChange,
    onNodesChange,
    onSelectionChange,
    onEdgeMouseEnter,
    onEdgeMouseLeave,
  } = useSelectCanvasData();
  const isValidConnection = useValidateConnection();

  const { onDrop, onDragOver, setReactFlowInstance, reactFlowInstance } =
    useHandleDrop();

  const {
    onNodeClick,
    clickedNode,
    formDrawerVisible,
    hideFormDrawer,
    singleDebugDrawerVisible,
    hideSingleDebugDrawer,
    showSingleDebugDrawer,
    chatVisible,
    runVisible,
    hideRunOrChatDrawer,
    showChatModal,
  } = useShowDrawer({
    drawerVisible,
    hideDrawer,
  });

  const {
    addEventList,
    setCurrentMessageId,
    currentEventListWithoutMessage,
    clearEventList,
    currentMessageId,
  } = useCacheChatLog();

  const { showLogSheet, logSheetVisible, hideLogSheet } = useShowLogSheet({
    setCurrentMessageId,
  });

  const { handleBeforeDelete } = useBeforeDelete();

  const { addCanvasNode, addNoteNode } = useAddNode(reactFlowInstance);

  const { ref, showImage, hideImage, imgVisible, mouse } = useMoveNote();

  const onPaneClick = useCallback(() => {
    hideFormDrawer();
    if (imgVisible) {
      addNoteNode(mouse);
      hideImage();
    }
  }, [addNoteNode, hideFormDrawer, hideImage, imgVisible, mouse]);

  useEffect(() => {
    if (!chatVisible) {
      clearEventList();
    }
  }, [chatVisible, clearEventList]);

  return (
    <div className={styles.canvasWrapper}>
      <svg
        xmlns="http://www.w3.org/2000/svg"
        style={{ position: 'absolute', top: 10, left: 0 }}
      >
        <defs>
          <marker
            fill="rgb(157 149 225)"
            id="logo"
            viewBox="0 0 40 40"
            refX="8"
            refY="5"
            markerUnits="strokeWidth"
            markerWidth="20"
            markerHeight="20"
            orient="auto-start-reverse"
          >
            <path d="M 0 0 L 10 5 L 0 10 z" />
          </marker>
        </defs>
      </svg>
      <AgentInstanceContext.Provider value={{ addCanvasNode }}>
        <ReactFlow
          connectionMode={ConnectionMode.Loose}
          nodes={nodes}
          onNodesChange={onNodesChange}
          edges={edges}
          onEdgesChange={onEdgesChange}
          fitView
          onConnect={onConnect}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          onDrop={onDrop}
          onDragOver={onDragOver}
          onNodeClick={onNodeClick}
          onPaneClick={onPaneClick}
          onInit={setReactFlowInstance}
          onSelectionChange={onSelectionChange}
          nodeOrigin={[0.5, 0]}
          isValidConnection={isValidConnection}
          onEdgeMouseEnter={onEdgeMouseEnter}
          onEdgeMouseLeave={onEdgeMouseLeave}
          className="h-full"
          colorMode="dark"
          defaultEdgeOptions={{
            type: 'buttonEdge',
            markerEnd: 'logo',
            style: {
              strokeWidth: 2,
              stroke: 'rgb(202 197 245)',
            },
            zIndex: 1001, // https://github.com/xyflow/xyflow/discussions/3498
          }}
          deleteKeyCode={['Delete', 'Backspace']}
          onBeforeDelete={handleBeforeDelete}
        >
          <Background />
          <Controls position={'bottom-center'} orientation="horizontal">
            <ControlButton>
              <Tooltip>
                <TooltipTrigger asChild>
                  <NotebookPen className="!fill-none" onClick={showImage} />
                </TooltipTrigger>
                <TooltipContent>{t('flow.note')}</TooltipContent>
              </Tooltip>
            </ControlButton>
          </Controls>
        </ReactFlow>
      </AgentInstanceContext.Provider>
      <NotebookPen
        className={cn('hidden absolute size-6', { block: imgVisible })}
        ref={ref}
      ></NotebookPen>
      {formDrawerVisible && (
        <AgentInstanceContext.Provider value={{ addCanvasNode }}>
          <FormSheet
            node={clickedNode}
            visible={formDrawerVisible}
            hideModal={hideFormDrawer}
            chatVisible={chatVisible}
            singleDebugDrawerVisible={singleDebugDrawerVisible}
            hideSingleDebugDrawer={hideSingleDebugDrawer}
            showSingleDebugDrawer={showSingleDebugDrawer}
          ></FormSheet>
        </AgentInstanceContext.Provider>
      )}
      {chatVisible && (
        <AgentChatContext.Provider value={{ showLogSheet }}>
          <AgentChatLogContext.Provider
            value={{ addEventList, setCurrentMessageId }}
          >
            <ChatSheet hideModal={hideRunOrChatDrawer}></ChatSheet>
          </AgentChatLogContext.Provider>
        </AgentChatContext.Provider>
      )}
      {runVisible && (
        <RunSheet
          hideModal={hideRunOrChatDrawer}
          showModal={showChatModal}
        ></RunSheet>
      )}
      {logSheetVisible && (
        <LogSheet
          hideModal={hideLogSheet}
          currentEventListWithoutMessage={currentEventListWithoutMessage}
          currentMessageId={currentMessageId}
        ></LogSheet>
      )}
    </div>
  );
}

export default AgentCanvas;
