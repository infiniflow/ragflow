import { useCallback } from 'react';
import ReactFlow, { Background, Controls, NodeMouseHandler } from 'reactflow';
import 'reactflow/dist/style.css';

import { NodeContextMenu, useHandleNodeContextMenu } from './context-menu';

import FlowDrawer from '../flow-drawer';
import {
  useHandleDrop,
  useHandleKeyUp,
  useSelectCanvasData,
  useShowDrawer,
} from '../hooks';
import { TextUpdaterNode } from './node';

import styles from './index.less';

const nodeTypes = { textUpdater: TextUpdaterNode };

interface IProps {
  sideWidth: number;
}

function FlowCanvas({ sideWidth }: IProps) {
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
        onPaneClick={onPaneClick}
        onDrop={onDrop}
        onDragOver={onDragOver}
        onNodeClick={onNodeClick}
        onInit={setReactFlowInstance}
        onKeyUp={handleKeyUp}
        onSelectionChange={onSelectionChange}
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
    </div>
  );
}

export default FlowCanvas;
