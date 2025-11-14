// src/components/ui/divider.tsx
import React from 'react';

type Direction = 'horizontal' | 'vertical';
type DividerType = 'horizontal' | 'vertical' | 'text';

interface DividerProps {
  direction?: Direction;
  type?: DividerType;
  text?: React.ReactNode;
  color?: string;
  margin?: string;
  className?: string;
}

const Divider: React.FC<DividerProps> = ({
  direction = 'horizontal',
  type = 'horizontal',
  text,
  color = 'border-muted-foreground/50',
  margin = 'my-4',
  className = '',
}) => {
  const baseClasses = 'flex items-center';
  const directionClass = direction === 'horizontal' ? 'flex-row' : 'flex-col';
  const colorClass = color.startsWith('border-') ? color : `border-${color}`;
  const marginClass = margin || '';
  const textClass = 'px-4 text-sm text-muted-foreground';

  // Default vertical style
  if (direction === 'vertical') {
    return (
      <div
        className={`h-full ${colorClass} border-l ${marginClass} ${className}`}
      >
        {type === 'text' && (
          <div className="transform -rotate-90 px-2 whitespace-nowrap">
            {text}
          </div>
        )}
      </div>
    );
  }

  // Horizontal with text
  if (type === 'text') {
    return (
      <div
        className={`${baseClasses} ${directionClass} ${marginClass} ${className}`}
      >
        <div className={`flex-1 ${colorClass} border-t`}></div>
        <div className={textClass}>{text}</div>
        <div className={`flex-1 ${colorClass} border-t`}></div>
      </div>
    );
  }

  // Default horizontal
  return (
    <div className={`${colorClass} border-t ${marginClass} ${className}`} />
  );
};

export default Divider;
