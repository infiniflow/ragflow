import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { useIsDarkTheme } from '../theme-provider';

import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import SvgIcon from '../svg-icon';
import { EmptyCardData, EmptyCardType, EmptyType } from './constant';
import { EmptyCardProps, EmptyProps } from './interface';

const EmptyIcon = ({ name, width }: { name: string; width?: number }) => {
  return (
    // <img
    //   className="h-20"
    //   src={isDarkTheme ? noDataIconDark : noDataIcon}
    //   alt={t('common.noData')}
    // />
    <SvgIcon name={name || 'empty/no-data-dark'} width={width || 42} />
  );
};

const Empty = (props: EmptyProps) => {
  const { className, children, type, text, iconWidth } = props;
  const isDarkTheme = useIsDarkTheme();

  const name = useMemo(() => {
    return isDarkTheme
      ? `empty/no-${type || EmptyType.Data}-dark`
      : `empty/no-${type || EmptyType.Data}-bri`;
  }, [isDarkTheme, type]);

  return (
    <div
      className={cn(
        'flex flex-col justify-center items-center text-center gap-2',
        className,
      )}
    >
      <EmptyIcon name={name} width={iconWidth} />

      {!children && (
        <div className="empty-text text-text-secondary text-sm">
          {text ||
            (type === 'data' ? t('common.noData') : t('common.noResults'))}
        </div>
      )}
      {children}
    </div>
  );
};

export default Empty;

export const EmptyCard = (props: EmptyCardProps) => {
  const { icon, className, children, title, description, style } = props;
  return (
    <div
      className={cn(
        'flex flex-col gap-3 items-start justify-start border border-dashed border-border-button rounded-md p-5 w-fit',
        className,
      )}
      style={style}
    >
      {icon}
      {title && <div className="text-text-primary text-base">{title}</div>}
      {description && (
        <div className="text-text-secondary text-sm">{description}</div>
      )}
      {children}
    </div>
  );
};

export const EmptyAppCard = (props: {
  type: EmptyCardType;
  onClick?: () => void;
  showIcon?: boolean;
  className?: string;
  isSearch?: boolean;
  size?: 'small' | 'large';
  children?: React.ReactNode;
}) => {
  const { type, showIcon, className, isSearch, children } = props;
  let defaultClass = '';
  let style = {};
  switch (props.size) {
    case 'small':
      style = { width: '256px' };
      defaultClass = 'mt-1';
      break;
    case 'large':
      style = { width: '480px' };
      defaultClass = 'mt-5';
      break;
    default:
      defaultClass = '';
      break;
  }
  return (
    <div onClick={isSearch ? undefined : props.onClick}>
      <EmptyCard
        icon={showIcon ? EmptyCardData[type].icon : undefined}
        title={
          isSearch ? EmptyCardData[type].notFound : EmptyCardData[type].title
        }
        className={cn('cursor-pointer ', className)}
        style={style}
        // description={EmptyCardData[type].description}
      >
        {!isSearch && !children && (
          <div
            className={cn(
              defaultClass,
              'flex items-center justify-start w-full',
            )}
          >
            <Plus size={24} />
          </div>
        )}
        {children}
      </EmptyCard>
    </div>
  );
};
