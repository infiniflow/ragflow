import { useCallback } from 'react';
import ReactFlow, {
  Background,
  ConnectionMode,
  Controls,
  NodeMouseHandler,
} from 'reactflow';
import 'reactflow/dist/style.css';

import { ButtonEdge } from './edge';

import FlowDrawer from '../flow-drawer';
import {
  useHandleDrop,
  useHandleKeyUp,
  useSelectCanvasData,
  useShowDrawer,
  useValidateConnection,
  useWatchNodeFormDataChange,
} from '../hooks';
import { RagNode } from './node';

import ChatDrawer from '../chat/drawer';
import styles from './index.less';
import { BeginNode } from './node/begin-node';
import { CategorizeNode } from './node/categorize-node';
import { LogicNode } from './node/logic-node';
import { RelevantNode } from './node/relevant-node';

const nodeTypes = {
  ragNode: RagNode,
  categorizeNode: CategorizeNode,
  beginNode: BeginNode,
  relevantNode: RelevantNode,
  logicNode: LogicNode,
};

const edgeTypes = {
  buttonEdge: ButtonEdge,
};

interface IProps {
  chatDrawerVisible: boolean;
  hideChatDrawer(): void;
}

function FlowCanvas({ chatDrawerVisible, hideChatDrawer }: IProps) {
  const {
    nodes,
    edges,
    onConnect,
    onEdgesChange,
    onNodesChange,
    onSelectionChange,
  } = useSelectCanvasData();
  const isValidConnection = useValidateConnection();

  const { drawerVisible, hideDrawer, showDrawer, clickedNode } =
    useShowDrawer();

  const onNodeClick: NodeMouseHandler = useCallback(
    (e, node) => {
      showDrawer(node);
    },
    [showDrawer],
  );

  const onPaneClick = useCallback(() => {
    hideDrawer();
  }, [hideDrawer]);

  const { onDrop, onDragOver, setReactFlowInstance } = useHandleDrop();

  const { handleKeyUp } = useHandleKeyUp();
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
        onKeyUp={handleKeyUp}
        onSelectionChange={onSelectionChange}
        nodeOrigin={[0.5, 0]}
        isValidConnection={isValidConnection}
        onChange={(...params) => {
          console.info('params:', ...params);
        }}
        defaultEdgeOptions={{
          type: 'buttonEdge',
          markerEnd: 'logo',
          // markerEnd: {
          //   type: MarkerType.ArrowClosed,
          //   color: 'rgb(157 149 225)',
          //   width: 20,
          //   height: 20,
          // },
          style: {
            // edge style
            strokeWidth: 2,
            stroke: 'rgb(202 197 245)',
          },
        }}
      >
        <Background />
        <Controls />
      </ReactFlow>
      <FlowDrawer
        node={clickedNode}
        visible={drawerVisible}
        hideModal={hideDrawer}
      ></FlowDrawer>
      {chatDrawerVisible && (
        <ChatDrawer
          visible={chatDrawerVisible}
          hideModal={hideChatDrawer}
        ></ChatDrawer>
      )}
    </div>
  );
}

export default FlowCanvas;
