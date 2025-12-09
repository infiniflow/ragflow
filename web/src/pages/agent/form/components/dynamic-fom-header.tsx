import { Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Plus } from 'lucide-react';
import { ReactNode } from 'react';

export type FormListHeaderProps = {
  label: ReactNode;
  tooltip?: string;
  onClick?: () => void;
  disabled?: boolean;
};

export function DynamicFormHeader({
  label,
  tooltip,
  onClick,
  disabled = false,
}: FormListHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <FormLabel tooltip={tooltip}>{label}</FormLabel>
      <Button
        variant={'ghost'}
        type="button"
        onClick={onClick}
        disabled={disabled}
      >
        <Plus />
      </Button>
    </div>
  );
}
