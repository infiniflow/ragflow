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

import { Button } from '@/components/ui/button';
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Activity, Settings2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { FutureAGIConfigurationDialog } from './futureagi-configuration-dialog';
import { useSaveFutureAGIConfiguration } from './use-save-futureagi-configuration';

export function FutureAGICard() {
  const {
    saveFutureAGIConfigurationOk,
    showSaveFutureAGIConfigurationModal,
    hideSaveFutureAGIConfigurationModal,
    saveFutureAGIConfigurationVisible,
    loading,
  } = useSaveFutureAGIConfiguration();
  const { t } = useTranslation();

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex justify-between">
          <div className="flex items-center gap-4">
            <Activity className="size-6" />
            Future AGI
          </div>
          <div className="flex gap-4 items-center">
            <Button size={'sm'} onClick={showSaveFutureAGIConfigurationModal}>
              <Settings2 />
              {t('setting.configuration')}
            </Button>
          </div>
        </CardTitle>
        <CardDescription>{t('setting.futureAGIDescription')}</CardDescription>
      </CardHeader>
      {saveFutureAGIConfigurationVisible && (
        <FutureAGIConfigurationDialog
          hideModal={hideSaveFutureAGIConfigurationModal}
          onOk={saveFutureAGIConfigurationOk}
          loading={loading}
        ></FutureAGIConfigurationDialog>
      )}
    </Card>
  );
}
