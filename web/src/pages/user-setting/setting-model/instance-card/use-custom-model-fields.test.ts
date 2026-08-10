/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 */

import { MODEL_FIELD_SCHEMA } from './use-custom-model-fields';

describe('MODEL_FIELD_SCHEMA', () => {
  it('requires at least one canonical model type', () => {
    const modelTypes = MODEL_FIELD_SCHEMA.find(
      (field) => field.name === 'model_types',
    );

    expect(modelTypes?.required).toBe(true);
    expect(modelTypes?.options?.map((option) => option.value)).toEqual(
      expect.arrayContaining(['chat', 'embedding', 'vision', 'asr']),
    );
    expect(modelTypes?.options?.map((option) => option.value)).not.toEqual(
      expect.arrayContaining(['image2text', 'speech2text']),
    );
  });
});
