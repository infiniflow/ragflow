import { useCallback } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MarkerType,
  NodeMouseHandler,
} from 'reactflow';
import 'reactflow/dist/style.css';

import { NodeContextMenu, useHandleNodeContextMenu } from './context-menu';
import { ButtonEdge } from './edge';

import FlowDrawer from '../flow-drawer';
import {
  useHandleDrop,
  useHandleKeyUp,
  useSelectCanvasData,
  useShowDrawer,
} from '../hooks';
import { TextUpdaterNode } from './node';

import ChatDrawer from '../chat/drawer';
import styles from './index.less';

const nodeTypes = { textUpdater: TextUpdaterNode };

const edgeTypes = {
  buttonEdge: ButtonEdge,
};

interface IProps {
  sideWidth: number;
  chatDrawerVisible: boolean;
  hideChatDrawer(): void;
}

function FlowCanvas({ sideWidth, chatDrawerVisible, hideChatDrawer }: IProps) {
  const {
    nodes,
    edges,
    onConnect,
    onEdgesChange,
    onNodesChange,
    onSelectionChange,
  } = useSelectCanvasData();

  const { ref, menu, onNodeContextMenu, onPaneClick } =
    useHandleNodeContextMenu(sideWidth);
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
        ref={ref}
        nodes={nodes}
        onNodesChange={onNodesChange}
        onNodeContextMenu={onNodeContextMenu}
        edges={edges}
        onEdgesChange={onEdgesChange}
        fitView
        onConnect={onConnect}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onPaneClick={onPaneClick}
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
        {Object.keys(menu).length > 0 && (
          <NodeContextMenu onClick={onPaneClick} {...(menu as any)} />
        )}
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
