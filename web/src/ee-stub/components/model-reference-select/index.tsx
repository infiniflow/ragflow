import type { ModelTreeSelectProps } from '@/components/model-tree-select';
import { forwardRef } from 'react';
import type { ForwardRefExoticComponent, RefAttributes } from 'react';

export type DirectModelSelectComponent = ForwardRefExoticComponent<
  ModelTreeSelectProps & RefAttributes<HTMLButtonElement>
>;

export interface ModelReferenceSelectProps extends ModelTreeSelectProps {
  directModelSelect: DirectModelSelectComponent;
}

export const ModelReferenceSelect = forwardRef<
  HTMLButtonElement,
  ModelReferenceSelectProps
>(function ModelReferenceSelect(
  { directModelSelect: DirectModelSelect, ...props },
  ref,
) {
  return <DirectModelSelect {...props} ref={ref} />;
});
