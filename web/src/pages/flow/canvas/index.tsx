import { useCallback } from 'react';
import ReactFlow, {
  Background,
  ConnectionMode,
  Controls,
  MarkerType,
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
} from '../hooks';
import { RagNode } from './node';

import ChatDrawer from '../chat/drawer';
import styles from './index.less';

const nodeTypes = { ragNode: RagNode };

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

  const { drawerVisible, hideDrawer, showDrawer, clickedNode } =
    useShowDrawer();

  const onNodeClick: NodeMouseHandler = useCallback(
    (e, node) => {
      showDrawer(node);
    },
    [showDrawer],
  );

  const { onDrop, onDragOver, setReactFlowInstance } = useHandleDrop();

  const { handleKeyUp } = useHandleKeyUp();

  return (
    <div className={styles.canvasWrapper}>
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
        onInit={setReactFlowInstance}
        onKeyUp={handleKeyUp}
        onSelectionChange={onSelectionChange}
        nodeOrigin={[0.5, 0]}
        onChange={(...params) => {
          console.info('params:', ...params);
        }}
        defaultEdgeOptions={{
          type: 'buttonEdge',
          markerEnd: {
            type: MarkerType.ArrowClosed,
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
      <ChatDrawer
        visible={chatDrawerVisible}
        hideModal={hideChatDrawer}
      ></ChatDrawer>
    </div>
  );
}

export default FlowCanvas;
