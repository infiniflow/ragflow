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

import type { TFunction } from 'i18next';
import { FormFieldType } from '@/components/dynamic-form';
import {
  DataSourceFormDefaultValues,
  DataSourceKey,
  generateDataSourceInfo,
  getDataSourceFieldsWithExtras,
} from './index';

const translate = ((key: string) => key) as TFunction;

describe('Xquik data source', () => {
  it('registers its catalog entry and defaults', () => {
    const info = generateDataSourceInfo(translate)[DataSourceKey.XQUIK];
    const defaults = DataSourceFormDefaultValues[DataSourceKey.XQUIK];

    expect(info.name).toBe('Xquik');
    expect(info.description).toBe('setting.xquikDescription');
    expect(defaults).toEqual({
      name: '',
      source: 'xquik',
      config: {
        query: '',
        query_type: 'Latest',
        page_size: 100,
        max_pages: 10,
        batch_size: 32,
        request_delay: 0.5,
        credentials: { xquik_api_key: '' },
      },
    });
  });

  it('keeps the API key secret and bounds page usage', () => {
    const fields = getDataSourceFieldsWithExtras(
      translate,
      DataSourceKey.XQUIK,
    ) as Array<{
      name: string;
      type?: FormFieldType;
      required?: boolean;
      validation?: { min?: number; max?: number };
    }>;
    const apiKey = fields.find(
      (field) => field.name === 'config.credentials.xquik_api_key',
    );
    const pageSize = fields.find((field) => field.name === 'config.page_size');
    const maxPages = fields.find((field) => field.name === 'config.max_pages');

    expect(apiKey?.type).toBe(FormFieldType.Password);
    expect(apiKey?.required).toBe(true);
    expect(pageSize?.validation).toMatchObject({ min: 1, max: 10000 });
    expect(maxPages?.validation).toMatchObject({ min: 1, max: 1000 });
  });
});

describe('Sitemap data source', () => {
  it('registers its catalog entry and defaults', () => {
    const info = generateDataSourceInfo(translate)[DataSourceKey.SITEMAP];
    const defaults = DataSourceFormDefaultValues[DataSourceKey.SITEMAP];

    expect(info.name).toBe('Sitemap');
    expect(info.description).toBe('setting.sitemapDescription');
    expect(defaults).toEqual({
      name: '',
      source: 'sitemap',
      config: {
        sitemap_url: '',
        url_filter: '',
        follow_pdf_links: false,
        restrict_pdf_to_domain: true,
        user_agent: '',
        batch_size: 10,
      },
    });
  });

  it('requires the sitemap URL and only shows the domain restriction when following PDFs', () => {
    const fields = getDataSourceFieldsWithExtras(
      translate,
      DataSourceKey.SITEMAP,
    ) as Array<{
      name: string;
      type?: FormFieldType;
      required?: boolean;
      validation?: { min?: number };
      shouldRender?: (values: any) => boolean;
    }>;
    const sitemapUrl = fields.find((f) => f.name === 'config.sitemap_url');
    const restrict = fields.find(
      (f) => f.name === 'config.restrict_pdf_to_domain',
    );
    const batchSize = fields.find((f) => f.name === 'config.batch_size');

    expect(sitemapUrl?.required).toBe(true);
    expect(restrict?.type).toBe(FormFieldType.Switch);
    expect(
      restrict?.shouldRender?.({ config: { follow_pdf_links: false } }),
    ).toBe(false);
    expect(
      restrict?.shouldRender?.({ config: { follow_pdf_links: true } }),
    ).toBe(true);
    expect(batchSize?.validation?.min).toBe(1);
  });
});
