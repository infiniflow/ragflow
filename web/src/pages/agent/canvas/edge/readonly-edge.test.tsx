import { render } from '@testing-library/react';
import { Position } from '@xyflow/react';
import { ReadonlyEdge } from './readonly-edge';

// Minimal EdgeProps payload; BaseEdge only needs the geometry to build the
// bezier path.
const baseEdgeProps: any = {
  id: 'edge-1',
  source: 'File',
  target: 'Parser:1',
  sourceX: 0,
  sourceY: 0,
  targetX: 100,
  targetY: 100,
  sourcePosition: Position.Right,
  targetPosition: Position.Left,
};

describe('ReadonlyEdge', () => {
  it('renders the edge path without any delete button', () => {
    const { container } = render(
      <svg>
        <ReadonlyEdge {...baseEdgeProps} />
      </svg>,
    );

    expect(container.querySelector('path')).not.toBeNull();
    expect(container.querySelector('button')).toBeNull();
  });

  it('highlights with the selected marker when selected', () => {
    const { container } = render(
      <svg>
        <ReadonlyEdge {...baseEdgeProps} selected />
      </svg>,
    );

    const path = container.querySelector('path');
    expect(path?.getAttribute('marker-end')).toBe('url(#selected-marker)');
  });
});
