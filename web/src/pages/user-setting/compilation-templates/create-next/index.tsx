import BackButton from '@/components/back-button';
import { CustomTimeline } from '@/components/originui/timeline';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { CompilationTemplateKind } from '@/constants/compilation';
import { Routes } from '@/routes';
import { useCallback, useMemo, useState } from 'react';
import { useFieldArray, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { BasicInfoStep } from './components/basic-info-step';
import { BlueprintsStep } from './components/blueprints-step';
import { TemplateConfiguration } from './components/template-configuration';
import { TemplateSidebar } from './components/template-sidebar';
import { useCreateNextCompilationTemplateGroup } from './hooks/use-create-next-compilation-template-group';

export default function CreateNextCompilationTemplate() {
  const { t } = useTranslation();
  const [activeStep, setActiveStep] = useState(1);
  const [selectedTemplateIndex, setSelectedTemplateIndex] = useState(0);

  const { form, kindOptions, builtins, onSubmit, isCreate, isLoading } =
    useCreateNextCompilationTemplateGroup();

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'templates',
  });

  const selectedKind = useWatch({
    control: form.control,
    name: `templates.${selectedTemplateIndex}.kind`,
  });

  const isArtifacts = selectedKind === CompilationTemplateKind.Artifacts;

  const timelineNodes = useMemo(
    () => [
      {
        id: 'basic-info',
        title: t('setting.basicInfo'),
        content: t('setting.basicInfoDescription'),
      },
      {
        id: 'configuration',
        title: t('setting.templateWizardConfiguration'),
        content: t('setting.templateWizardConfigurationDescription'),
      },
      ...(isArtifacts
        ? [
            {
              id: 'blueprints',
              title: t('setting.blueprints'),
              content: t('setting.blueprintsDescription'),
            },
          ]
        : []),
    ],
    [isArtifacts, t],
  );

  const handleNext = useCallback(async () => {
    if (activeStep === 1) {
      const valid = await form.trigger(['name', 'description']);
      if (valid) setActiveStep(2);
    } else if (activeStep === 2) {
      const valid = await form.trigger(`templates.${selectedTemplateIndex}`);
      if (!valid) return;
      if (isArtifacts) {
        setActiveStep(3);
      } else {
        form.handleSubmit(onSubmit)();
      }
    }
  }, [activeStep, form, isArtifacts, onSubmit, selectedTemplateIndex]);

  const handleBack = useCallback(() => {
    setActiveStep(1);
  }, []);

  return (
    <section className="h-full flex flex-col bg-bg-base">
      <header className="shrink-0 px-5 py-4 border-b border-border-button flex gap-3">
        <BackButton
          to={`${Routes.UserSetting}${Routes.CompilationTemplates}`}
        />
        <h2 className="font-medium text-text-secondary">
          {isCreate
            ? t('setting.addTemplateGroup')
            : t('setting.editTemplateGroup')}
        </h2>
      </header>

      <div className="shrink-0 px-5 py-4 border-b border-border-button">
        <CustomTimeline
          nodes={timelineNodes}
          activeStep={activeStep}
          onStepChange={(step) => {
            // Allow clicking back to completed steps only.
            if (step < activeStep) {
              setActiveStep(step);
            }
          }}
          orientation="horizontal"
        />
      </div>

      <Form {...form}>
        <form className="flex-1 min-h-0 flex">
          {activeStep === 2 ? (
            <ResizablePanelGroup direction="horizontal" className="flex-1">
              <ResizablePanel defaultSize={25} minSize={20} maxSize={40}>
                <TemplateSidebar
                  form={form}
                  fields={fields}
                  append={append}
                  remove={remove}
                  kindOptions={kindOptions}
                  selectedTemplateIndex={selectedTemplateIndex}
                  onSelectTemplate={setSelectedTemplateIndex}
                />
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel className="min-h-0 flex flex-col">
                <TemplateConfiguration
                  form={form}
                  builtins={builtins}
                  kindOptions={kindOptions}
                  selectedTemplateIndex={selectedTemplateIndex}
                  onNext={handleNext}
                  onBack={handleBack}
                  isArtifacts={isArtifacts}
                  isLoading={isLoading}
                />
              </ResizablePanel>
            </ResizablePanelGroup>
          ) : (
            <div className="flex-1 min-h-0 flex flex-col">
              {activeStep === 1 && (
                <>
                  <BasicInfoStep />
                  <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-end">
                    <Button type="button" onClick={handleNext}>
                      {t('common.next')}
                    </Button>
                  </footer>
                </>
              )}

              {activeStep === 3 && isArtifacts && (
                <BlueprintsStep
                  form={form}
                  selectedTemplateIndex={selectedTemplateIndex}
                  onBack={() => setActiveStep(2)}
                  onSave={form.handleSubmit(onSubmit)}
                  isLoading={isLoading}
                />
              )}
            </div>
          )}
        </form>
      </Form>
    </section>
  );
}
