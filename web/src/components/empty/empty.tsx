import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { useIsDarkTheme } from '../theme-provider';

import { Plus } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import SvgIcon from '../svg-icon';
import { EmptyCardData, EmptyCardType, EmptyType } from './constant';
import { EmptyCardProps, EmptyProps } from './interface';

const EmptyIcon = ({ name, width }: { name: string; width?: number }) => {
  return <SvgIcon name={name || 'empty/no-data-dark'} width={width || 42} />;
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
  const { icon, className, children, title, description, style, ...restProps } =
    props;
  return (
    <article
      className={cn(
        'flex flex-col gap-3 rounded-md border border-dashed border-border-button p-5',
        'w-full items-center justify-center text-center',
        'md:w-fit md:items-start md:justify-start md:text-left',
        className,
      )}
      style={style}
      {...restProps}
    >
      {icon}
      {title && <div className="text-sm text-text-primary">{title}</div>}
      {description && (
        <p className="text-sm text-text-secondary">{description}</p>
      )}
      {children}
    </article>
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
  testId?: string;
  tabIndex?: number;
}) => {
  const { type, showIcon, className, isSearch, children, testId, tabIndex } =
    props;
  const { t } = useTranslation();
  let defaultClass = '';
  let style: React.CSSProperties | undefined;
  const cardData = EmptyCardData[type];
  const title = t(cardData.titleKey);
  const notFound = t(cardData.notFoundKey);

  switch (props.size) {
    case 'small':
      style = { width: '256px' };
      defaultClass = 'mt-1';
      break;
    case 'large':
      defaultClass = 'mt-5';
      break;
    default:
      defaultClass = '';
      break;
  }

  return (
    <div className="flex w-full justify-center px-5 md:px-0">
      <EmptyCard
        onClick={isSearch ? undefined : props.onClick}
        data-testid={testId}
        tabIndex={tabIndex ?? (isSearch ? undefined : 0)}
        icon={showIcon ? cardData.icon : undefined}
        title={isSearch ? notFound : title}
        className={cn(
          !isSearch && 'cursor-pointer',
          props.size === 'large' && 'p-14',
          className,
          'w-full max-w-[480px] md:max-w-none',
          props.size === 'large' && 'md:w-[480px]',
          props.size === 'small' && 'max-w-64',
        )}
        style={style}
      >
        {!isSearch && !children && (
          <div
            className={cn(
              defaultClass,
              'flex w-full items-center justify-center md:justify-start',
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
