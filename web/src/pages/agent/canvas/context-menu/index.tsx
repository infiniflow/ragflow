import { NodeMouseHandler, useReactFlow } from '@xyflow/react';
import { useCallback, useRef, useState } from 'react';

import styles from './index.module.less';

export interface INodeContextMenu {
  id: string;
  top: number;
  left: number;
  right?: number;
  bottom?: number;
  [key: string]: unknown;
}

export function NodeContextMenu({
  id,
  top,
  left,
  right,
  bottom,
  ...props
}: INodeContextMenu) {
  const { getNode, setNodes, addNodes, setEdges } = useReactFlow();

  const duplicateNode = useCallback(() => {
    const node = getNode(id);
    const position = {
      x: node?.position?.x || 0 + 50,
      y: node?.position?.y || 0 + 50,
    };

    addNodes({
      ...(node || {}),
      data: node?.data,
      selected: false,
      dragging: false,
      id: `${node?.id}-copy`,
      position,
    });
  }, [id, getNode, addNodes]);

  const deleteNode = useCallback(() => {
    setNodes((nodes) => nodes.filter((node) => node.id !== id));
    setEdges((edges) => edges.filter((edge) => edge.source !== id));
  }, [id, setNodes, setEdges]);

  return (
    <div
      style={{ top, left, right, bottom }}
      className={styles.contextMenu}
      {...props}
    >
      <p style={{ margin: '0.5em' }}>
        <small>node: {id}</small>
      </p>
      <button onClick={duplicateNode} type={'button'}>
        duplicate
      </button>
      <button onClick={deleteNode} type={'button'}>
        delete
      </button>
    </div>
  );
}

/*  @deprecated
 */
export const useHandleNodeContextMenu = (sideWidth: number) => {
  const [menu, setMenu] = useState<INodeContextMenu>({} as INodeContextMenu);
  const ref = useRef<any>(null);

  const onNodeContextMenu: NodeMouseHandler = useCallback(
    (event, node) => {
      // Prevent native context menu from showing
      event.preventDefault();

      // Calculate position of the context menu. We want to make sure it
      // doesn't get positioned off-screen.
      const pane = ref.current?.getBoundingClientRect();
      // setMenu({
      //   id: node.id,
      //   top: event.clientY < pane.height - 200 ? event.clientY : 0,
      //   left: event.clientX < pane.width - 200 ? event.clientX : 0,
      //   right: event.clientX >= pane.width - 200 ? pane.width - event.clientX : 0,
      //   bottom:
      //     event.clientY >= pane.height - 200 ? pane.height - event.clientY : 0,
      // });

      setMenu({
        id: node.id,
        top: event.clientY - 144,
        left: event.clientX - sideWidth,
        // top: event.clientY < pane.height - 200 ? event.clientY - 72 : 0,
        // left: event.clientX < pane.width - 200 ? event.clientX : 0,
      });
    },
    [sideWidth],
  );

  // Close the context menu if it's open whenever the window is clicked.
  const onPaneClick = useCallback(
    () => setMenu({} as INodeContextMenu),
    [setMenu],
  );

  return { onNodeContextMenu, menu, onPaneClick, ref };
};
