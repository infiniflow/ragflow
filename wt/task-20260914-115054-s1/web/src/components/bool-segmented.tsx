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
