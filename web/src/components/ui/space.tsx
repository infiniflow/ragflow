// src/components/ui/space.tsx
import React from 'react';

type Direction = 'horizontal' | 'vertical';
type Size = 'small' | 'middle' | 'large';

interface SpaceProps {
  direction?: Direction;
  size?: Size;
  align?: string;
  justify?: string;
  wrap?: boolean;
  className?: string;
  children: React.ReactNode;
}

const sizeClasses: Record<Size, string> = {
  small: 'gap-2',
  middle: 'gap-4',
  large: 'gap-8',
};

const directionClasses: Record<Direction, string> = {
  horizontal: 'flex-row',
  vertical: 'flex-col',
};

const Space: React.FC<SpaceProps> = ({
  direction = 'horizontal',
  size = 'middle',
  align,
  justify,
  wrap = false,
  className = '',
  children,
}) => {
  const baseClasses = 'flex';
  const directionClass = directionClasses[direction];
  const sizeClass = sizeClasses[size];
  const alignClass = align ? `items-${align}` : '';
  const justifyClass = justify ? `justify-${justify}` : '';
  const wrapClass = wrap ? 'flex-wrap' : '';

  const classes = [
    baseClasses,
    directionClass,
    sizeClass,
    alignClass,
    justifyClass,
    wrapClass,
    className,
  ].join(' ');

  return <div className={classes}>{children}</div>;
};

export default Space;
