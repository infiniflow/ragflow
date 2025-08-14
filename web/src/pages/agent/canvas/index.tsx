import { useIsDarkTheme, useTheme } from '@/components/theme-provider';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useSetModalState } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import {
  Connection,
  ConnectionMode,
  ControlButton,
  Controls,
  NodeTypes,
  Position,
  ReactFlow,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { NotebookPen } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChatSheet } from '../chat/chat-sheet';
import { AgentBackground } from '../components/background';
import {
  AgentChatContext,
  AgentChatLogContext,
  AgentInstanceContext,
  HandleContext,
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
import { useDropdownManager } from './context';

import {
  useHideFormSheetOnNodeDeletion,
  useShowDrawer,
  useShowLogSheet,
} from '../hooks/use-show-drawer';
import { LogSheet } from '../log-sheet';
import RunSheet from '../run-sheet';
import { ButtonEdge } from './edge';
import styles from './index.less';
import { RagNode } from './node';
import { AgentNode } from './node/agent-node';
import { BeginNode } from './node/begin-node';
import { CategorizeNode } from './node/categorize-node';
import { InnerNextStepDropdown } from './node/dropdown/next-step-dropdown';
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

export const nodeTypes: NodeTypes = {
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
  // emailNode: EmailNode,
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
    onConnect: originalOnConnect,
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
    showFormDrawer,
  } = useShowDrawer({
    drawerVisible,
    hideDrawer,
  });

  const {
    addEventList,
    setCurrentMessageId,
    currentEventListWithoutMessageById,
    clearEventList,
    currentMessageId,
  } = useCacheChatLog();

  const { showLogSheet, logSheetVisible, hideLogSheet } = useShowLogSheet({
    setCurrentMessageId,
  });
  const [lastSendLoading, setLastSendLoading] = useState(false);

  const { handleBeforeDelete } = useBeforeDelete();

  const { addCanvasNode, addNoteNode } = useAddNode(reactFlowInstance);

  const { ref, showImage, hideImage, imgVisible, mouse } = useMoveNote();

  const { theme } = useTheme();

  useEffect(() => {
    if (!chatVisible) {
      clearEventList();
    }
  }, [chatVisible, clearEventList]);
  const setLastSendLoadingFunc = (loading: boolean, messageId: string) => {
    if (messageId === currentMessageId) {
      setLastSendLoading(loading);
    } else {
      setLastSendLoading(false);
    }
  };

  const isDarkTheme = useIsDarkTheme();

  useHideFormSheetOnNodeDeletion({ hideFormDrawer });

  const { visible, hideModal, showModal } = useSetModalState();
  const [dropdownPosition, setDropdownPosition] = useState({ x: 0, y: 0 });

  const isConnectedRef = useRef(false);
  const connectionStartRef = useRef<{
    nodeId: string;
    handleId: string;
  } | null>(null);

  const preventCloseRef = useRef(false);

  const { setActiveDropdown, clearActiveDropdown } = useDropdownManager();

  const onPaneClick = useCallback(() => {
    hideFormDrawer();
    if (visible && !preventCloseRef.current) {
      hideModal();
      clearActiveDropdown();
    }
    if (imgVisible) {
      addNoteNode(mouse);
      hideImage();
    }
  }, [
    hideFormDrawer,
    visible,
    hideModal,
    imgVisible,
    addNoteNode,
    mouse,
    hideImage,
    clearActiveDropdown,
  ]);

  const onConnect = (connection: Connection) => {
    originalOnConnect(connection);
    isConnectedRef.current = true;
  };

  const OnConnectStart = (event: any, params: any) => {
    isConnectedRef.current = false;

    if (params && params.nodeId && params.handleId) {
      connectionStartRef.current = {
        nodeId: params.nodeId,
        handleId: params.handleId,
      };
    } else {
      connectionStartRef.current = null;
    }
  };

  const OnConnectEnd = (event: MouseEvent | TouchEvent) => {
    if ('clientX' in event && 'clientY' in event) {
      const { clientX, clientY } = event;
      setDropdownPosition({ x: clientX, y: clientY });
      if (!isConnectedRef.current) {
        setActiveDropdown('drag');
        showModal();
        preventCloseRef.current = true;
        setTimeout(() => {
          preventCloseRef.current = false;
        }, 300);
      }
    }
  };

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
      <AgentInstanceContext.Provider value={{ addCanvasNode, showFormDrawer }}>
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
          onConnectStart={OnConnectStart}
          onConnectEnd={OnConnectEnd}
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
          colorMode={theme}
          defaultEdgeOptions={{
            type: 'buttonEdge',
            markerEnd: 'logo',
            style: {
              strokeWidth: 1,
              stroke: isDarkTheme
                ? 'rgba(91, 93, 106, 1)'
                : 'rgba(151, 154, 171, 1)',
            },
            zIndex: 1001, // https://github.com/xyflow/xyflow/discussions/3498
          }}
          deleteKeyCode={['Delete', 'Backspace']}
          onBeforeDelete={handleBeforeDelete}
        >
          <AgentBackground></AgentBackground>
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
        {visible && (
          <HandleContext.Provider
            value={{
              nodeId: connectionStartRef.current?.nodeId || '',
              id: connectionStartRef.current?.handleId || '',
              type: 'source',
              position: Position.Right,
              isFromConnectionDrag: true,
            }}
          >
            <InnerNextStepDropdown
              hideModal={() => {
                hideModal();
                clearActiveDropdown();
              }}
              position={dropdownPosition}
            >
              <span></span>
            </InnerNextStepDropdown>
          </HandleContext.Provider>
        )}
      </AgentInstanceContext.Provider>
      <NotebookPen
        className={cn('hidden absolute size-6', { block: imgVisible })}
        ref={ref}
      ></NotebookPen>
      {formDrawerVisible && (
        <AgentInstanceContext.Provider
          value={{ addCanvasNode, showFormDrawer }}
        >
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
        <AgentChatContext.Provider
          value={{ showLogSheet, setLastSendLoadingFunc }}
        >
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
          currentEventListWithoutMessageById={
            currentEventListWithoutMessageById
          }
          currentMessageId={currentMessageId}
          sendLoading={lastSendLoading}
        ></LogSheet>
      )}
    </div>
  );
}

export default AgentCanvas;
