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
  OnConnectEnd,
  Position,
  ReactFlow,
  ReactFlowInstance,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { NotebookPen } from 'lucide-react';
import { memo, useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentInstanceContext, HandleContext } from '../context';

import FormSheet from '../form-sheet/next';
import { useSelectCanvasData, useValidateConnection } from '../hooks';
import { useAddNode } from '../hooks/use-add-node';
import { useBeforeDelete } from '../hooks/use-before-delete';
import { useMoveNote } from '../hooks/use-move-note';
import { useDropdownManager } from './context';

import { AgentBackground } from '@/components/canvas/background';
import Spotlight from '@/components/spotlight';
import { useRunDataflow } from '../hooks/use-run-dataflow';
import {
  useHideFormSheetOnNodeDeletion,
  useShowDrawer,
} from '../hooks/use-show-drawer';
import RunSheet from '../run-sheet';
import useGraphStore from '../store';
import { ButtonEdge } from './edge';
import styles from './index.less';
import { RagNode } from './node';
import { BeginNode } from './node/begin-node';
import { NextStepDropdown } from './node/dropdown/next-step-dropdown';
import { ExtractorNode } from './node/extractor-node';
import NoteNode from './node/note-node';
import ParserNode from './node/parser-node';
import { SplitterNode } from './node/splitter-node';
import TokenizerNode from './node/tokenizer-node';

export const nodeTypes: NodeTypes = {
  ragNode: RagNode,
  beginNode: BeginNode,
  noteNode: NoteNode,
  parserNode: ParserNode,
  tokenizerNode: TokenizerNode,
  splitterNode: SplitterNode,
  contextNode: ExtractorNode,
};

const edgeTypes = {
  buttonEdge: ButtonEdge,
};

interface IProps {
  drawerVisible: boolean;
  hideDrawer(): void;
  showLogSheet(): void;
}

function DataFlowCanvas({ drawerVisible, hideDrawer, showLogSheet }: IProps) {
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

  const [reactFlowInstance, setReactFlowInstance] =
    useState<ReactFlowInstance<any, any>>();

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
    showFormDrawer,
  } = useShowDrawer({
    drawerVisible,
    hideDrawer,
  });

  const { handleBeforeDelete } = useBeforeDelete();

  const { addCanvasNode, addNoteNode } = useAddNode(reactFlowInstance);

  const { ref, showImage, hideImage, imgVisible, mouse } = useMoveNote();

  const { theme } = useTheme();

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

  const { hasChildNode } = useGraphStore((state) => state);

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

  const { run, loading: running } = useRunDataflow(
    showLogSheet!,
    hideRunOrChatDrawer,
  );

  const onConnect = (connection: Connection) => {
    originalOnConnect(connection);
    isConnectedRef.current = true;
  };

  const onConnectStart = (event: any, params: any) => {
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

  const onConnectEnd: OnConnectEnd = (event, connectionState) => {
    const target = event.target as HTMLElement;
    const nodeId = connectionState.fromNode?.id;

    // Events triggered by Handle are directly interrupted
    if (
      target?.classList.contains('react-flow__handle') ||
      (nodeId && hasChildNode(nodeId))
    ) {
      return;
    }

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
    <div className={cn(styles.canvasWrapper, 'px-5 pb-5')}>
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
          onConnectStart={onConnectStart}
          onConnectEnd={onConnectEnd}
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
          <Spotlight className="z-0" opcity={0.7} coverage={70} />
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
            <NextStepDropdown
              hideModal={() => {
                hideModal();
                clearActiveDropdown();
              }}
              position={dropdownPosition}
              nodeId={connectionStartRef.current?.nodeId || ''}
            >
              <span></span>
            </NextStepDropdown>
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
      {runVisible && (
        <RunSheet
          hideModal={hideRunOrChatDrawer}
          run={run}
          loading={running}
        ></RunSheet>
      )}
    </div>
  );
}

export default memo(DataFlowCanvas);
