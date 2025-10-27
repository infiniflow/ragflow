import { cn } from '@/lib/utils';
import { isValidElement, PropsWithChildren, ReactNode } from 'react';
import './index.less';

type CardContainerProps = { className?: string } & PropsWithChildren;

export function CardSineLineContainer({
  children,
  className,
}: CardContainerProps) {
  const flattenChildren = (children: ReactNode): ReactNode[] => {
    const result: ReactNode[] = [];

    const traverse = (child: ReactNode) => {
      if (Array.isArray(child)) {
        child.forEach(traverse);
      } else if (isValidElement(child) && child.props.children) {
        result.push(child);
      } else {
        result.push(child);
      }
    };

    traverse(children);
    return result;
  };
  const childArray = flattenChildren(children);
  const childCount = childArray.length;
  console.log(childArray, childCount);

  return (
    <section
      className={cn(
        'grid gap-6 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6 single',
        className,
      )}
    >
      {children}
    </section>
  );
}
