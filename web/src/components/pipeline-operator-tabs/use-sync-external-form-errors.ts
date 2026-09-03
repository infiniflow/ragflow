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

import { useEffect, useRef } from 'react';
import { FieldErrors, UseFormReturn } from 'react-hook-form';

type FlattenedError = {
  path: string;
  message: string;
};

/**
 * Flattens a react-hook-form errors tree into dot-separated field paths,
 * e.g. { setups: { 0: { fields: { message } } } } → 'setups.0.fields'.
 */
function flattenFieldErrors(
  errors: FieldErrors | undefined,
  parentPath = '',
): FlattenedError[] {
  if (!errors || typeof errors !== 'object') {
    return [];
  }

  return Object.entries(errors).flatMap(([key, value]) => {
    if (!value || typeof value !== 'object') {
      return [];
    }
    const path = parentPath ? `${parentPath}.${key}` : key;
    if ('message' in value && typeof value.message === 'string') {
      return [{ path, message: value.message }];
    }
    return flattenFieldErrors(value as FieldErrors, path);
  });
}

/**
 * Mirrors validation errors produced by an outer form (whose schema validates
 * parser_config on submit) onto the matching fields of an operator form, so
 * the messages render on the fields via FormMessage. Errors whose outer
 * issue is gone are cleared; errors for fixed fields are also cleared by the
 * operator form's own resolver, which validates the same schema on change.
 */
export function useSyncExternalFormErrors(
  form: UseFormReturn<any>,
  externalErrors?: FieldErrors,
) {
  const previousPathsRef = useRef<string[]>([]);

  useEffect(() => {
    const entries = flattenFieldErrors(externalErrors);
    const nextPaths = entries.map((x) => x.path);

    previousPathsRef.current
      .filter((path) => !nextPaths.includes(path))
      .forEach((path) => form.clearErrors(path));
    entries.forEach(({ path, message }) => {
      form.setError(path, { message });
    });

    previousPathsRef.current = nextPaths;
  }, [form, externalErrors]);
}
