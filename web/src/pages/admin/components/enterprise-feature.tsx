import { IS_ENTERPRISE } from '../utils';

export default function EnterpriseFeature({
  children,
}: {
  children: React.ReactNode | (() => React.ReactNode);
}) {
  return IS_ENTERPRISE
    ? typeof children === 'function'
      ? children()
      : children
    : null;
}
