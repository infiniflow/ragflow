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

import { BackendVariant } from '@/utils/backend-variant';
import { memo } from 'react';
import { INextOperatorForm } from '../../interface';
import GoExtractorForm from './go-form';
import PythonExtractorForm from './python-form';

export { FormSchema } from './go-form';
export type { ExtractorFormSchemaType } from './go-form';

// The Go backend supports the modularized extractor config (per-feature
// sub-tabs with nested params), while the Python backend only understands the
// legacy flat fields — pick the matching form per backend.
const ExtractorForm = (props: INextOperatorForm) => (
  <BackendVariant
    go={<GoExtractorForm {...props} />}
    python={<PythonExtractorForm {...props} />}
  />
);

export default memo(ExtractorForm);
