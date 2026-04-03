import { EmptyType } from './constant';

export type EmptyProps = {
  className?: string;
  children?: React.ReactNode;
  type?: EmptyType;
  text?: string;
  iconWidth?: number;
};

export type EmptyCardProps = {
  icon?: React.ReactNode;
  className?: string;
  children?: React.ReactNode;
  title?: string;
  description?: string;
  style?: React.CSSProperties;
};
