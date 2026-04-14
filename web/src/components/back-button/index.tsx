import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { ArrowBigLeft } from 'lucide-react';
import React from 'react';
import { useNavigate } from 'react-router';
import { Button } from '../ui/button';

interface BackButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  to?: string;
}

const BackButton: React.FC<BackButtonProps> = ({
  to,
  className,
  children,
  ...props
}) => {
  const navigate = useNavigate();

  const handleClick = () => {
    if (to) {
      navigate(to);
    } else {
      navigate(-1);
    }
  };

  return (
    <Button
      variant="ghost"
      className={cn(
        'gap-2 bg-bg-card border border-border-default hover:bg-border-button hover:text-text-primary',
        className,
      )}
      onClick={handleClick}
      {...props}
    >
      <ArrowBigLeft className="h-4 w-4" />
      {children || t('common.back')}
    </Button>
  );
};

export default BackButton;
