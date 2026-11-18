import { cn } from '@/lib/utils';

interface PipelineVersionLabelProps {
  title: string;
  alias?: string;
  className?: string;
  aliasClassName?: string;
}

// The "version title + alias" line used everywhere a published pipeline
// version is displayed: the parser tree select's dropdown rows and closed
// trigger, and the dataset table's parse column. The title truncates first;
// the alias stays visible as secondary text, width-capped via aliasClassName
// so an over-long alias ends in an ellipsis instead of overflowing (the
// closed trigger caps it at 30% of the trigger width; the dropdown row and
// the parse column cap it at fixed widths).
export function PipelineVersionLabel({
  title,
  alias,
  className,
  aliasClassName,
}: PipelineVersionLabelProps) {
  return (
    <span className={cn('flex min-w-0 items-center gap-1.5', className)}>
      <span className="truncate text-text-primary">{title}</span>
      {alias ? (
        <span
          className={cn(
            'shrink-0 truncate text-text-secondary',
            aliasClassName,
          )}
        >
          {alias}
        </span>
      ) : null}
    </span>
  );
}
