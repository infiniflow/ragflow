import { useIsDarkTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';

export default function ThemeLogo(
  props: React.ImgHTMLAttributes<HTMLImageElement>,
) {
  const isDark = useIsDarkTheme();

  return (
    <img
      {...props}
      className={cn('object-contain', props.className)}
      src={isDark ? '/logo-dark.png' : '/logo-light.png'}
      alt={props.alt ?? 'MetaGross-AI logo'}
    />
  );
}
