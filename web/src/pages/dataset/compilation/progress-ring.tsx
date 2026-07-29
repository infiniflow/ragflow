import { cn } from '@/lib/utils';
import { toFixed } from '@/utils/common-util';

interface IProgressRingProps {
  percent: number;
  failed?: boolean;
  size?: number;
}

const DefaultRingSize = 240;
const RingStrokeWidth = 16;

export function ProgressRing({
  percent,
  failed = false,
  size = DefaultRingSize,
}: IProgressRingProps) {
  const radius = (size - RingStrokeWidth * 2) / 2;
  const circumference = 2 * Math.PI * radius;
  const strokeDashoffset = circumference - (percent / 100) * circumference;

  return (
    <div
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(percent)}
      className="relative inline-flex items-center justify-center"
      style={{ width: size, height: size }}
    >
      <svg
        className="rotate-[-90deg] transform"
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        fill="none"
        stroke="currentColor"
      >
        <circle
          className="text-border-button"
          strokeWidth={RingStrokeWidth}
          cx={size / 2}
          cy={size / 2}
          r={radius}
        />
        <circle
          className={cn(
            'transition-[stroke-dashoffset] duration-300 ease-linear',
            failed ? 'text-state-error' : 'text-accent-primary',
          )}
          strokeWidth={RingStrokeWidth}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={strokeDashoffset}
          cx={size / 2}
          cy={size / 2}
          r={radius}
        />
      </svg>
      <span className="absolute text-5xl font-medium text-text-primary">
        {`${toFixed(percent, 0)}%`}
      </span>
    </div>
  );
}
