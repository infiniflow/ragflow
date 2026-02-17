import { omit } from 'lodash';
import { Segmented, SegmentedProps } from './ui/segmented';

export function BoolSegmented({ ...props }: Omit<SegmentedProps, 'options'>) {
  return (
    <Segmented
      options={
        [
          { value: true, label: 'True' },
          { value: false, label: 'False' },
        ] as any
      }
      sizeType="sm"
      itemClassName="justify-center flex-1"
      {...omit(props, 'options')}
    ></Segmented>
  );
}
