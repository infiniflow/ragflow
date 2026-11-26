import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

type PageCloseButtonProps = {
  onClick: () => void;
  className?: string;
};

export function PageCloseButton({ onClick, className }: PageCloseButtonProps) {
  const { t } = useTranslation();

  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={t('common.close')}
      title={t('common.close')}
      onClick={onClick}
      className={cn('absolute right-2 top-4 z-10 md:right-6', className)}
    >
      <X className="size-6" />
    </Button>
  );
}
