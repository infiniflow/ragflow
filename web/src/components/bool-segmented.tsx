/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  Segmented,
  SegmentedLabeledOption,
  SegmentedProps,
  SegmentedValue,
} from './ui/segmented';

const BoolOptions: SegmentedLabeledOption[] = [
  { value: true as unknown as SegmentedValue, label: 'True' },
  { value: false as unknown as SegmentedValue, label: 'False' },
];

interface BoolSegmentedProps extends Omit<
  SegmentedProps,
  'options' | 'value' | 'defaultValue' | 'onChange'
> {
  value?: boolean;
  defaultValue?: boolean;
  onChange?: (value: boolean) => void;
}

export function BoolSegmented({
  value,
  defaultValue,
  onChange,
  ...props
}: BoolSegmentedProps) {
  return (
    <Segmented
      options={BoolOptions}
      sizeType="sm"
      itemClassName="justify-center flex-1"
      value={value as unknown as SegmentedValue}
      defaultValue={defaultValue as unknown as SegmentedValue}
      onChange={onChange as SegmentedProps['onChange']}
      {...props}
    ></Segmented>
  );
}
