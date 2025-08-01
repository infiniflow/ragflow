import { useIsDarkTheme } from '@/components/theme-provider';
import { Background } from '@xyflow/react';

export function AgentBackground() {
  const isDarkTheme = useIsDarkTheme();

  return (
    <Background
      color={isDarkTheme ? 'rgba(255,255,255,0.15)' : '#A8A9B3'}
      bgColor={isDarkTheme ? 'rgba(11, 11, 12, 1)' : 'rgba(0, 0, 0, 0.05)'}
    />
  );
}
