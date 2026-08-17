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

import React from 'react';
import { useTranslation } from 'react-i18next';
import { isRouteErrorResponse, useRouteError } from 'react-router';

interface FallbackComponentProps {
  error?: Error;
  reset?: () => void;
}

const FallbackComponent: React.FC<FallbackComponentProps> = ({
  error: errorProp,
  reset,
}) => {
  const { t } = useTranslation();
  const routeError = useRouteError();
  const error =
    errorProp ?? (routeError instanceof Error ? routeError : undefined);

  let routeErrorDataStr = '';
  if (isRouteErrorResponse(routeError)) {
    if (typeof routeError.data === 'string') {
      routeErrorDataStr = routeError.data;
    } else if (routeError.data == null) {
      routeErrorDataStr = 'no body';
    } else {
      try {
        routeErrorDataStr = JSON.stringify(routeError.data);
      } catch {
        routeErrorDataStr = String(routeError.data);
      }
    }
  }

  const errorMessage = isRouteErrorResponse(routeError)
    ? `${routeError.status} ${routeError.statusText}${routeErrorDataStr ? `: ${routeErrorDataStr}` : ''}`
    : (error?.toString() ?? (routeError ? String(routeError) : undefined));

  return (
    <div style={{ padding: '20px', textAlign: 'center' }}>
      <h2>{t('error_boundary.title', 'Something went wrong')}</h2>
      <p>
        {t(
          'error_boundary.description',
          'Sorry, an error occurred while loading the page.',
        )}
      </p>
      {errorMessage && (
        <details open className="mt-4 whitespace-pre-wrap">
          <summary>{t('error_boundary.details', 'Error details')}</summary>
          {errorMessage}
        </details>
      )}
      <div style={{ marginTop: '16px' }}>
        <button
          onClick={() => window.location.reload()}
          style={{
            marginRight: '12px',
            padding: '8px 16px',
            backgroundColor: '#1890ff',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          {t('error_boundary.reload', 'Reload Page')}
        </button>
        {reset && (
          <button
            onClick={reset}
            style={{
              padding: '8px 16px',
              backgroundColor: '#52c41a',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            {t('error_boundary.retry', 'Retry')}
          </button>
        )}
      </div>
    </div>
  );
};

export default FallbackComponent;
