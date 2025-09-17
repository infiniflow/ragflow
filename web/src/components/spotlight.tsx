import { useIsDarkTheme } from '@/components/theme-provider';
import React from 'react';

interface SpotlightProps {
  className?: string;
  opcity?: number;
  coverage?: number;
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
}) => {
  const isDark = useIsDarkTheme();
  const rgb = isDark ? '255, 255, 255' : '194, 221, 243';
  return (
    <div
      className={`absolute inset-0  opacity-80 ${className} rounded-lg`}
      style={{
        backdropFilter: 'blur(30px)',
        zIndex: -1,
      }}
    >
      <div
        className="absolute inset-0"
        style={{
          background: `radial-gradient(circle at 50% 190%, rgba(${rgb},${opcity}) 0%, rgba(${rgb},0) ${coverage}%)`,
          pointerEvents: 'none',
        }}
      ></div>
    </div>
  );
};

export default Spotlight;
