import { cn } from '@/lib/utils';
import * as AvatarPrimitive from '@radix-ui/react-avatar';
import { forwardRef, memo, useEffect, useRef, useState } from 'react';
import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar';

const PREDEFINED_COLORS = [
  { from: '#4F6DEE', to: '#67BDF9' },
  { from: '#38A04D', to: '#93DCA2' },
  { from: '#C35F2B', to: '#EDB395' },
  { from: '#633897', to: '#CBA1FF' },
];

const getStringHash = (str: string): number => {
  if (typeof str !== 'string') return 0;

  const normalized = str.trim().toLowerCase();
  let hash = 104729;
  const seed = 0x9747b28c;

  for (let i = 0; i < normalized.length; i++) {
    hash ^= seed ^ normalized.charCodeAt(i);
    hash = (hash << 13) | (hash >>> 19);
    hash = (hash * 5 + 0x52dce72d) | 0;
  }

  return Math.abs(hash);
};

const getColorForName = (name: string): { from: string; to: string } => {
  const hash = getStringHash(name);
  const index = hash % PREDEFINED_COLORS.length;
  return PREDEFINED_COLORS[index];
};

export const RAGFlowAvatar = memo(
  forwardRef<
    React.ElementRef<typeof AvatarPrimitive.Root>,
    React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Root> & {
      name?: string;
      avatar?: string;
      isPerson?: boolean;
    }
  >(({ name, avatar, isPerson = false, className, ...props }, ref) => {
    // Generate initial letter logic
    const getInitials = (name?: string) => {
      if (typeof name !== 'string' || !name) return '';
      const parts = name?.trim().split(/\s+/);
      if (parts.length === 1) {
        return parts[0][0].toUpperCase();
      }
      return parts[0][0].toUpperCase();
    };

    const initials = getInitials(name);
    const { from, to } = name
      ? getColorForName(name)
      : { from: 'hsl(0, 0%, 30%)', to: 'hsl(0, 0%, 80%)' };

    const fallbackRef = useRef<HTMLElement>(null);
    const [fontSize, setFontSize] = useState('0.875rem');

    // Calculate font size
    const calculateFontSize = () => {
      if (fallbackRef.current) {
        const containerWidth = fallbackRef.current.offsetWidth;
        const newSize = containerWidth * 0.6;
        setFontSize(`${newSize}px`);
      }
    };

    useEffect(() => {
      calculateFontSize();

      if (fallbackRef.current) {
        const resizeObserver = new ResizeObserver(() => {
          calculateFontSize();
        });

        resizeObserver.observe(fallbackRef.current);

        return () => {
          if (fallbackRef.current) {
            resizeObserver.unobserve(fallbackRef.current);
          }
          resizeObserver.disconnect();
        };
      }
    }, []);

    return (
      <Avatar
        ref={ref}
        {...props}
        className={cn(className, { 'rounded-md': !isPerson })}
      >
        <AvatarImage src={avatar} />
        <AvatarFallback
          ref={(node) => {
            fallbackRef.current = node;
            calculateFontSize();
          }}
          className={cn(
            'bg-gradient-to-b',
            `from-[${from}] to-[${to}]`,
            'flex items-center justify-center',
            'text-white ',
            { 'rounded-md': !isPerson },
          )}
          style={{
            backgroundImage: `linear-gradient(to bottom, ${from}, ${to})`,
            fontSize: fontSize,
          }}
        >
          {initials}
        </AvatarFallback>
      </Avatar>
    );
  }),
);

RAGFlowAvatar.displayName = 'RAGFlowAvatar';
