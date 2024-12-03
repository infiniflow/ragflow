import { useSetModalState } from '@/hooks/common-hooks';
import { useCallback, useEffect } from 'react';
import ReactFlow, {
  Background,
  ConnectionMode,
  Controls,
  NodeMouseHandler,
} from 'reactflow';
import 'reactflow/dist/style.css';
import ChatDrawer from '../chat/drawer';
import { Operator } from '../constant';
import FormDrawer from '../flow-drawer';
import {
  useGetBeginNodeDataQuery,
  useHandleDrop,
  useSelectCanvasData,
  useShowFormDrawer,
  useValidateConnection,
  useWatchNodeFormDataChange,
} from '../hooks';
import { BeginQuery } from '../interface';
import RunDrawer from '../run-drawer';
import { ButtonEdge } from './edge';
import styles from './index.less';
import { RagNode } from './node';
import { BeginNode } from './node/begin-node';
import { CategorizeNode } from './node/categorize-node';
import { GenerateNode } from './node/generate-node';
import { InvokeNode } from './node/invoke-node';
import { KeywordNode } from './node/keyword-node';
import { LogicNode } from './node/logic-node';
import { MessageNode } from './node/message-node';
import NoteNode from './node/note-node';
import { RelevantNode } from './node/relevant-node';
import { RetrievalNode } from './node/retrieval-node';
import { RewriteNode } from './node/rewrite-node';
import { SwitchNode } from './node/switch-node';
import { TemplateNode } from './node/template-node';

const nodeTypes = {
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
};

const edgeTypes = {
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
  const {
    visible: runVisible,
    showModal: showRunModal,
    hideModal: hideRunModal,
  } = useSetModalState();
  const {
    visible: chatVisible,
    showModal: showChatModal,
    hideModal: hideChatModal,
  } = useSetModalState();

  const { formDrawerVisible, hideFormDrawer, showFormDrawer, clickedNode } =
    useShowFormDrawer();

  const onPaneClick = useCallback(() => {
    hideFormDrawer();
  }, [hideFormDrawer]);

  const { onDrop, onDragOver, setReactFlowInstance } = useHandleDrop();

  useWatchNodeFormDataChange();

  const hideRunOrChatDrawer = useCallback(() => {
    hideChatModal();
    hideRunModal();
    hideDrawer();
  }, [hideChatModal, hideDrawer, hideRunModal]);

  const onNodeClick: NodeMouseHandler = useCallback(
    (e, node) => {
      if (node.data.label !== Operator.Note) {
        hideRunOrChatDrawer();
        showFormDrawer(node);
      }
    },
    [hideRunOrChatDrawer, showFormDrawer],
  );

  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();

  useEffect(() => {
    if (drawerVisible) {
      const query: BeginQuery[] = getBeginNodeDataQuery();
      if (query.length > 0) {
        showRunModal();
        hideChatModal();
      } else {
        showChatModal();
        hideRunModal();
      }
    }
  }, [
    hideChatModal,
    hideRunModal,
    showChatModal,
    showRunModal,
    drawerVisible,
    getBeginNodeDataQuery,
  ]);

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
        onChangeCapture={(...params) => {
          console.info('onChangeCapture:', ...params);
        }}
        onChange={(...params) => {
          console.info('params:', ...params);
        }}
        defaultEdgeOptions={{
          type: 'buttonEdge',
          markerEnd: 'logo',
          style: {
            strokeWidth: 2,
            stroke: 'rgb(202 197 245)',
          },
        }}
        deleteKeyCode={['Delete', 'Backspace']}
      >
        <Background />
        <Controls />
      </ReactFlow>
      {formDrawerVisible && (
        <FormDrawer
          node={clickedNode}
          visible={formDrawerVisible}
          hideModal={hideFormDrawer}
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
    </div>
  );
}

export default FlowCanvas;
