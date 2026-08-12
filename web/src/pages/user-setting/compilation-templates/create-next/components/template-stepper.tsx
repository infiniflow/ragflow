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
  Stepper,
  StepperDescription,
  StepperIndicator,
  StepperItem,
  StepperNav,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
} from '@/components/reui/stepper';
import { cn } from '@/lib/utils';
import { Cog, FileText, Palette } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

interface TemplateStepperProps {
  activeStep: number;
  isArtifacts: boolean;
  onStepChange: (step: number) => void;
}

export function TemplateStepper({
  activeStep,
  isArtifacts,
  onStepChange,
}: TemplateStepperProps) {
  const { t } = useTranslation();

  const steps = useMemo(
    () => [
      {
        id: 'basic-info',
        title: t('setting.basicInfo'),
        description: t('setting.basicInfoDescription'),
        icon: FileText,
      },
      {
        id: 'configuration',
        title: t('setting.templateWizardConfiguration'),
        description: t('setting.templateWizardConfigurationDescription'),
        icon: Cog,
      },
      ...(isArtifacts
        ? [
            {
              id: 'blueprints',
              title: t('setting.blueprints'),
              description: t('setting.blueprintsDescription'),
              icon: Palette,
            },
          ]
        : []),
    ],
    [isArtifacts, t],
  );

  return (
    <Stepper
      value={activeStep}
      onValueChange={onStepChange}
      className="flex justify-center"
    >
      <StepperNav className="justify-center max-w-4xl">
        {steps.map((step, index) => {
          const stepNumber = index + 1;
          const isActive = stepNumber === activeStep;
          const Icon = step.icon;
          const isLast = index === steps.length - 1;

          return (
            <StepperItem
              key={step.id}
              step={stepNumber}
              className="items-start flex-1 relative"
              disabled={stepNumber > activeStep}
            >
              <StepperTrigger type="button" className="flex flex-col gap-1">
                <StepperIndicator
                  className={cn(
                    'size-8 border !bg-bg-card !text-text-primary !border-border-button',
                    isActive && '!border-accent-primary !text-accent-primary',
                  )}
                >
                  <Icon className="size-4" />
                </StepperIndicator>
                <StepperTitle
                  className={cn(
                    'mt-2 text-center text-sm font-medium leading-none',
                    isActive && 'text-accent-primary',
                  )}
                >
                  {step.title}
                </StepperTitle>
                <StepperDescription
                  className={'text-center text-text-secondary'}
                >
                  {step.description}
                </StepperDescription>
              </StepperTrigger>
              {!isLast && (
                <StepperSeparator className="group-data-[state=completed]/step:bg-primary absolute inset-x-0 top-3.5 left-[calc(50%+1.4rem)] m-0 group-data-[orientation=horizontal]/stepper-nav:w-[calc(100%-3rem+0.225rem)] group-data-[orientation=horizontal]/stepper-nav:flex-none" />
              )}
            </StepperItem>
          );
        })}
      </StepperNav>
    </Stepper>
  );
}
