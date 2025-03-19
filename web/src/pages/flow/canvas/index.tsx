import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Background,
  ConnectionMode,
  ControlButton,
  Controls,
  NodeTypes,
  ReactFlow,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { Book, FolderInput, FolderOutput } from 'lucide-react';
import ChatDrawer from '../chat/drawer';
import FormDrawer from '../flow-drawer';
import {
  useHandleDrop,
  useSelectCanvasData,
  useValidateConnection,
  useWatchNodeFormDataChange,
} from '../hooks';
import { useBeforeDelete } from '../hooks/use-before-delete';
import { useHandleExportOrImportJsonFile } from '../hooks/use-export-json';
import { useOpenDocument } from '../hooks/use-open-document';
import { useShowDrawer } from '../hooks/use-show-drawer';
import JsonUploadModal from '../json-upload-modal';
import RunDrawer from '../run-drawer';
import { ButtonEdge } from './edge';
import styles from './index.less';
import { RagNode } from './node';
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
  emailNode: EmailNode,
  group: IterationNode,
  iterationStartNode: IterationStartNode,
};

export const edgeTypes = {
  buttonEdge: ButtonEdge,
};

interface IProps {
  drawerVisible: boolean;
  hideDrawer(): void;
}

function FlowCanvas({ drawerVisible, hideDrawer }: IProps) {
  const {
    nodes,
    edges,
    onConnect,
    onEdgesChange,
    onNodesChange,
    onSelectionChange,
  } = useSelectCanvasData();
  const isValidConnection = useValidateConnection();

  const { onDrop, onDragOver, setReactFlowInstance } = useHandleDrop();

  const {
    handleExportJson,
    handleImportJson,
    fileUploadVisible,
    onFileUploadOk,
    hideFileUploadModal,
  } = useHandleExportOrImportJsonFile();

  const openDocument = useOpenDocument();

  const {
    onNodeClick,
    onPaneClick,
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

  const { handleBeforeDelete } = useBeforeDelete();

  useWatchNodeFormDataChange();

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
        <Controls className="text-black !flex-col-reverse">
          <ControlButton onClick={handleImportJson}>
            <Tooltip>
              <TooltipTrigger asChild>
                <FolderInput className="!fill-none" />
              </TooltipTrigger>
              <TooltipContent>Import</TooltipContent>
            </Tooltip>
          </ControlButton>
          <ControlButton onClick={handleExportJson}>
            <Tooltip>
              <TooltipTrigger asChild>
                <FolderOutput className="!fill-none" />
              </TooltipTrigger>
              <TooltipContent>Export</TooltipContent>
            </Tooltip>
          </ControlButton>
          <ControlButton onClick={openDocument}>
            <Tooltip>
              <TooltipTrigger asChild>
                <Book className="!fill-none" />
              </TooltipTrigger>
              <TooltipContent>Document</TooltipContent>
            </Tooltip>
          </ControlButton>
        </Controls>
      </ReactFlow>
      {formDrawerVisible && (
        <FormDrawer
          node={clickedNode}
          visible={formDrawerVisible}
          hideModal={hideFormDrawer}
          singleDebugDrawerVisible={singleDebugDrawerVisible}
          hideSingleDebugDrawer={hideSingleDebugDrawer}
          showSingleDebugDrawer={showSingleDebugDrawer}
        ></FormDrawer>
      )}
      {chatVisible && (
        <ChatDrawer
          visible={chatVisible}
          hideModal={hideRunOrChatDrawer}
        ></ChatDrawer>
      )}

      {runVisible && (
        <RunDrawer
          hideModal={hideRunOrChatDrawer}
          showModal={showChatModal}
        ></RunDrawer>
      )}
      {fileUploadVisible && (
        <JsonUploadModal
          onOk={onFileUploadOk}
          visible={fileUploadVisible}
          hideModal={hideFileUploadModal}
        ></JsonUploadModal>
      )}
    </div>
  );
}

export default FlowCanvas;
