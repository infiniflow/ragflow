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

import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { RefObject } from 'react';
import { DRAFT_INSTANCE_SENTINEL, DraftModeCardProps } from '../interface';
import { ModelsSection } from '../models-section';
import VerifyButton from '../verify-button';
import { InstanceNameSection } from './instance-name-section';

/**
 * The draft (unsaved) variant of the provider instance card.
 *
 * Renders the instance name input section at the top (without a Save
 * button - the parent drives save through the imperative ref API),
 * followed by the form fields, verify button, and per-instance models
 * section. All fields are editable from the start; there is no
 * fieldset lock - the user can fill in everything and submit via the
 * top-of-page Save button.
 */
export function DraftModeCard({
  formFields,
  formDefaultValues,
  formRef,
  handleVerify,
  handleDelete,
  handleInstanceModelsEdited,
  providerName,
  instanceName,
  instance,
  modelInfoRef,
  draftName,
  setDraftName,
}: DraftModeCardProps) {
  return (
    <div className="px-2 py-3 flex flex-col gap-4">
      <InstanceNameSection
        draftName={draftName}
        setDraftName={setDraftName}
        handleDelete={handleDelete}
      />

      <DynamicForm.Root
        key={`${providerName}-${instanceName}-true`}
        ref={formRef as RefObject<DynamicFormRef>}
        fields={formFields}
        onSubmit={() => undefined}
        defaultValues={formDefaultValues}
        labelClassName="font-normal"
      />

      <div className="pt-3">
        <VerifyButton
          onVerify={handleVerify}
          isAbsolute={false}
          formRef={formRef}
        />
      </div>

      <div className="pt-3">
        <ModelsSection
          providerName={providerName}
          instanceName={instanceName || DRAFT_INSTANCE_SENTINEL}
          instance={instance}
          hideActions={false}
          hideIfEmpty={false}
          getFormValues={() => formRef.current?.getValues?.() ?? {}}
          onInstanceModelsChange={(info) => {
            modelInfoRef.current = info;
          }}
          onInstanceModelsEdited={handleInstanceModelsEdited}
        />
      </div>
    </div>
  );
}
