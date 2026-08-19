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

import BackButton from '@/components/back-button';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { CompilationTemplateKind } from '@/constants/compilation';
import { useFetchCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { Routes } from '@/routes';
import { useCallback, useMemo } from 'react';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';

import { BlueprintSection } from './components/blueprint-section';
import { TemplateConfiguration } from './components/template-configuration';
import { useEditNextCompilationTemplateGroup } from './hooks/use-edit-next-compilation-template-group';

const SelectedTemplateIndex = 0;

const agentsUrl = Routes.Agents;

export default function EditNextCompilationTemplate() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const navigateToAgents = useCallback(() => {
    navigate(agentsUrl);
  }, [navigate]);

  const { form, kindOptions, builtins, onSubmit, isCreate, isLoading } =
    useEditNextCompilationTemplateGroup({
      onSuccess: navigateToAgents,
    });
  const { data: group } = useFetchCompilationTemplateGroup();

  const selectedKind = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.kind`,
  });

  const isArtifacts = selectedKind === CompilationTemplateKind.Artifacts;

  const handleSave = useMemo(
    () => form.handleSubmit(onSubmit),
    [form, onSubmit],
  );

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3 items-center">
        <BackButton to={agentsUrl} />
        <h2 className="font-medium text-text-secondary">
          {isCreate
            ? t('knowledgeCompilation.addTemplateGroup')
            : group?.name || t('knowledgeCompilation.editTemplateGroup')}
        </h2>
      </header>

      <Form {...form}>
        <form className="flex-1 min-h-0 flex flex-col" onSubmit={handleSave}>
          <TemplateConfiguration
            form={form}
            builtins={builtins}
            kindOptions={kindOptions}
            selectedTemplateIndex={SelectedTemplateIndex}
          >
            {isArtifacts && (
              <BlueprintSection
                form={form}
                selectedTemplateIndex={SelectedTemplateIndex}
                builtins={builtins}
              />
            )}
          </TemplateConfiguration>

          <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-end gap-5">
            <Button type="button" variant="outline" onClick={navigateToAgents}>
              {t('common.back')}
            </Button>
            <Button type="submit" loading={isLoading}>
              {t('common.save')}
            </Button>
          </footer>
        </form>
      </Form>
    </section>
  );
}
