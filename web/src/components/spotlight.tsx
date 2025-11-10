import { useIsDarkTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import { parseColorToRGB } from '@/utils/common-util';
import React from 'react';

interface SpotlightProps {
  className?: string;
  opcity?: number;
  coverage?: number;
  X?: string;
  Y?: string;
  color?: string;
}
/**
 *
 * @param opcity 0~1 default 0.5
 * @param coverage 0~100 default 60
 * @returns
 */
const Spotlight: React.FC<SpotlightProps> = ({
  className,
  opcity = 0.5,
  coverage = 60,
  X = '50%',
  Y = '190%',
  color,
}) => {
  const isDark = useIsDarkTheme();
  let realColor: [number, number, number] | undefined = undefined;
  if (color) {
    realColor = parseColorToRGB(color);
  }
  const rgb = realColor
    ? realColor.join(',')
    : isDark
      ? '255, 255, 255'
      : '194, 221, 243';
  return (
    <div
      className={cn('absolute inset-0 opacity-80 rounded-lg', className)}
      style={{
        backdropFilter: 'blur(30px)',
        zIndex: -1,
      }}
    >
      <div
        className="absolute inset-0"
        style={{
          background: `radial-gradient(circle at ${X} ${Y}, rgba(${rgb},${opcity}) 0%, rgba(${rgb},0) ${coverage}%)`,
          pointerEvents: 'none',
        }}
      ></div>
    </div>
  );
};

export default Spotlight;
