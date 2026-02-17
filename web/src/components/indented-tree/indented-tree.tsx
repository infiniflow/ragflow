import { Rect } from '@antv/g';
import {
  Badge,
  BaseBehavior,
  BaseNode,
  CommonEvent,
  ExtensionCategory,
  Graph,
  NodeEvent,
  Point,
  Polyline,
  PolylineStyleProps,
  register,
  subStyleProps,
  treeToGraphData,
} from '@antv/g6';
import { TreeData } from '@antv/g6/lib/types';
import isEmpty from 'lodash/isEmpty';
import React, { useCallback, useEffect, useRef } from 'react';
import { ErrorBoundary, FallbackProps } from 'react-error-boundary';
import { useIsDarkTheme } from '../theme-provider';

const rootId = 'root';

const COLORS = [
  '#5B8FF9',
  '#F6BD16',
  '#5AD8A6',
  '#945FB9',
  '#E86452',
  '#6DC8EC',
  '#FF99C3',
  '#1E9493',
  '#FF9845',
  '#5D7092',
];

const TreeEvent = {
  COLLAPSE_EXPAND: 'collapse-expand',
  WHEEL: 'canvas:wheel',
};

class IndentedNode extends BaseNode {
  static defaultStyleProps = {
    ports: [
      {
        key: 'in',
        placement: 'right-bottom',
      },
      {
        key: 'out',
        placement: 'left-bottom',
      },
    ],
  } as any;

  constructor(options: any) {
    Object.assign(options.style, IndentedNode.defaultStyleProps);
    super(options);
  }

  get childrenData() {
    return this.attributes.context?.model.getChildrenData(this.id);
  }

  getKeyStyle(attributes: any) {
    const [width, height] = this.getSize(attributes);
    const keyStyle = super.getKeyStyle(attributes);
    return {
      width,
      height,
      ...keyStyle,
      fill: 'transparent',
    };
  }

  drawKeyShape(attributes: any, container: any) {
    const keyStyle = this.getKeyStyle(attributes);
    return this.upsert('key', Rect, keyStyle, container);
  }

  getLabelStyle(attributes: any) {
    if (attributes.label === false || !attributes.labelText) return false;
    return subStyleProps(this.getGraphicStyle(attributes), 'label') as any;
  }

  drawIconArea(attributes: any, container: any) {
    const [, h] = this.getSize(attributes);
    const iconAreaStyle = {
      fill: 'transparent',
      height: 30,
      width: 12,
      x: -6,
      y: h,
      zIndex: -1,
    };
    this.upsert('icon-area', Rect, iconAreaStyle, container);
  }

  forwardEvent(target: any, type: any, listener: any) {
    if (target && !Reflect.has(target, '__bind__')) {
      Reflect.set(target, '__bind__', true);
      target.addEventListener(type, listener);
    }
  }

  getCountStyle(attributes: any) {
    const { collapsed, color } = attributes;
    if (collapsed) {
      const [, height] = this.getSize(attributes);
      return {
        backgroundFill: color,
        cursor: 'pointer',
        fill: '#fff',
        fontSize: 8,
        padding: [0, 10],
        text: `${this.childrenData?.length}`,
        textAlign: 'center',
        y: height + 8,
      };
    }

    return false;
  }

  drawCountShape(attributes: any, container: any) {
    const countStyle = this.getCountStyle(attributes);
    const btn = this.upsert('count', Badge, countStyle as any, container);

    this.forwardEvent(btn, CommonEvent.CLICK, (event: any) => {
      event.stopPropagation();
      attributes.context.graph.emit(TreeEvent.COLLAPSE_EXPAND, {
        id: this.id,
        collapsed: false,
      });
    });
  }

  isShowCollapse(attributes: any) {
    return (
      !attributes.collapsed &&
      Array.isArray(this.childrenData) &&
      this.childrenData?.length > 0
    );
  }

  getCollapseStyle(attributes: any) {
    const { showIcon, color } = attributes;
    if (!this.isShowCollapse(attributes)) return false;
    const [, height] = this.getSize(attributes);
    return {
      visibility: showIcon ? 'visible' : 'hidden',
      backgroundFill: color,
      backgroundHeight: 12,
      backgroundWidth: 12,
      cursor: 'pointer',
      fill: '#fff',
      fontFamily: 'iconfont',
      fontSize: 8,
      text: '\ue6e4',
      textAlign: 'center',
      x: -1, // half of edge line width
      y: height + 8,
    };
  }

  drawCollapseShape(attributes: any, container: any) {
    const iconStyle = this.getCollapseStyle(attributes);
    const btn = this.upsert(
      'collapse-expand',
      Badge,
      iconStyle as any,
      container,
    );

    this.forwardEvent(btn, CommonEvent.CLICK, (event: any) => {
      event.stopPropagation();
      attributes.context.graph.emit(TreeEvent.COLLAPSE_EXPAND, {
        id: this.id,
        collapsed: !attributes.collapsed,
      });
    });
  }

  getAddStyle(attributes: any) {
    const { collapsed, showIcon } = attributes;
    if (collapsed) return false;
    const [, height] = this.getSize(attributes);
    const color = '#ddd';
    const lineWidth = 1;

    return {
      visibility: showIcon ? 'visible' : 'hidden',
      backgroundFill: '#fff',
      backgroundHeight: 12,
      backgroundLineWidth: lineWidth,
      backgroundStroke: color,
      backgroundWidth: 12,
      cursor: 'pointer',
      fill: color,
      fontFamily: 'iconfont',
      text: '\ue664',
      textAlign: 'center',
      x: -1,
      y: height + (this.isShowCollapse(attributes) ? 22 : 8),
    };
  }

  render(attributes = this.parsedAttributes, container = this) {
    super.render(attributes, container);

    this.drawCountShape(attributes, container);

    this.drawIconArea(attributes, container);
    this.drawCollapseShape(attributes, container);
  }
}

class IndentedEdge extends Polyline {
  getControlPoints(
    attributes: Required<PolylineStyleProps>,
    sourcePoint: Point,
    targetPoint: Point,
  ) {
    const [sx] = sourcePoint;
    const [, ty] = targetPoint;
    return [[sx, ty]] as any;
  }
}

class CollapseExpandTree extends BaseBehavior {
  constructor(context: any, options: any) {
    super(context, options);
    this.bindEvents();
  }

  update(options: any) {
    this.unbindEvents();
    super.update(options);
    this.bindEvents();
  }

  bindEvents() {
    const { graph } = this.context;

    graph.on(NodeEvent.POINTER_ENTER, this.showIcon);
    graph.on(NodeEvent.POINTER_LEAVE, this.hideIcon);
    graph.on(TreeEvent.COLLAPSE_EXPAND, this.onCollapseExpand);
  }

  unbindEvents() {
    const { graph } = this.context;

    graph.off(NodeEvent.POINTER_ENTER, this.showIcon);
    graph.off(NodeEvent.POINTER_LEAVE, this.hideIcon);
    graph.off(TreeEvent.COLLAPSE_EXPAND, this.onCollapseExpand);
  }

  status = 'idle';

  showIcon = (event: any) => {
    this.setIcon(event, true);
  };

  hideIcon = (event: any) => {
    this.setIcon(event, false);
  };

  setIcon = (event: any, show: boolean) => {
    if (this.status !== 'idle') return;
    const { target } = event;
    const id = target.id;
    const { graph, element } = this.context;
    graph.updateNodeData([{ id, style: { showIcon: show } }]);
    element?.draw({ animation: false, silence: true });
  };

  onCollapseExpand = async (event: any) => {
    this.status = 'busy';
    const { id, collapsed } = event;
    const { graph } = this.context;
    if (collapsed) await graph.collapseElement(id);
    else await graph.expandElement(id);
    this.status = 'idle';
  };
}

register(ExtensionCategory.NODE, 'indented', IndentedNode);
register(ExtensionCategory.EDGE, 'indented', IndentedEdge);
register(
  ExtensionCategory.BEHAVIOR,
  'collapse-expand-tree',
  CollapseExpandTree,
);

interface IProps {
  data: TreeData;
  show: boolean;
  style?: React.CSSProperties;
}

function fallbackRender({ error }: FallbackProps) {
  // Call resetErrorBoundary() to reset the error boundary and retry the render.

  return (
    <div role="alert">
      <p>Something went wrong:</p>
      <pre style={{ color: 'red' }}>{error.message}</pre>
    </div>
  );
}

const IndentedTree = ({ data, show, style = {} }: IProps) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const graphRef = useRef<Graph | null>(null);
  const assignIds = React.useCallback(function assignIds(
    node: TreeData,
    parentId: string = '',
    index = 0,
  ) {
    if (!node.id) node.id = parentId ? `${parentId}-${index}` : 'root';
    if (node.children) {
      node.children.forEach((child, idx) => assignIds(child, node.id, idx));
    }
  }, []);
  const isDark = useIsDarkTheme();
  const render = useCallback(
    async (data: TreeData) => {
      const graph: Graph = new Graph({
        container: containerRef.current!,
        x: 60,
        node: {
          type: 'indented',
          style: {
            size: (d) => [d.id.length * 6 + 10, 20],
            labelBackground: (datum) => datum.id === rootId,
            labelBackgroundRadius: 0,
            labelBackgroundFill: '#576286',
            labelFill: isDark ? '#fff' : '#333',
            // labelFill: (datum) => (datum.id === rootId ? '#fff' : '#666'),
            labelText: (d) => d.style?.labelText || d.id,
            labelTextAlign: (datum) =>
              datum.id === rootId ? 'center' : 'left',
            labelTextBaseline: 'top',
            color: (datum: any) => {
              const depth = graph.getAncestorsData(datum.id, 'tree').length - 1;
              return COLORS[depth % COLORS.length] || '#576286';
            },
          },
          state: {
            selected: {
              lineWidth: 0,
              labelFill: '#40A8FF',
              labelBackground: true,
              labelFontWeight: 'normal',
              labelBackgroundFill: '#e8f7ff',
              labelBackgroundRadius: 10,
            },
          },
        },
        edge: {
          type: 'indented',
          style: {
            radius: 16,
            lineWidth: 2,
            sourcePort: 'out',
            targetPort: 'in',
            stroke: (datum: any) => {
              const depth = graph.getAncestorsData(datum.source, 'tree').length;
              return COLORS[depth % COLORS.length] || 'black';
            },
          },
        },
        layout: {
          type: 'indented',
          direction: 'LR',
          isHorizontal: true,
          indent: 40,
          getHeight: () => 20,
          getVGap: () => 10,
        },
        behaviors: [
          'scroll-canvas',
          'collapse-expand-tree',
          {
            type: 'click-select',
            enable: (event: any) =>
              event.targetType === 'node' && event.target.id !== rootId,
          },
        ],
      });

      if (graphRef.current) {
        graphRef.current.destroy();
      }

      graphRef.current = graph;

      assignIds(data);

      graph?.setData(treeToGraphData(data));

      graph?.render();
    },
    [assignIds],
  );

  useEffect(() => {
    if (!isEmpty(data)) {
      render(data);
    }
  }, [render, data]);

  return (
    <ErrorBoundary fallbackRender={fallbackRender}>
      <div
        id="tree"
        ref={containerRef}
        style={{
          width: '90vw',
          height: '80vh',
          display: show ? 'block' : 'none',
          ...style,
        }}
      />
    </ErrorBoundary>
  );
};

export default IndentedTree;
