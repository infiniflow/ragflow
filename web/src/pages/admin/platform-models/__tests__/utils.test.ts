import { getProviderConfig } from '@/components/model-provider-schema/field-config';

import {
  baseUrlFieldOf,
  buildInstanceFormValues,
  buildInstanceSubmission,
  hasCredentialInput,
  isHiddenSavedAddress,
  missingTestFields,
  pickSafeSettings,
  withClearedSettings,
  withoutUnsavedDefaults,
} from '../utils';

const detail: AdminService.PlatformInstanceDetail = {
  id: 'instance',
  provider_id: 'provider',
  instance_name: 'default',
  status: 'active',
  base_url: '',
  base_url_configured: true,
  api_key_configured: true,
  api_key_config: { group_id: 'group-1' },
  credential_status: { api_key: true },
};

describe('platform instance form values', () => {
  it('never seeds stored credentials into the form', () => {
    const values = buildInstanceFormValues(
      getProviderConfig('MiniMax'),
      detail,
    );
    expect(values.api_key).toBe('');
    expect(values.group_id).toBe('group-1');
    // A hidden saved endpoint stays blank so saving keeps it.
    expect(values.base_url).toBeUndefined();
  });
});

describe('platform instance submission', () => {
  it('uses the provider transform for structured credentials', () => {
    const submission = buildInstanceSubmission(
      getProviderConfig('MiniMax'),
      {
        instance_name: ' main ',
        api_key: 'new-key',
        group_id: 'g',
        base_url: 'https://api.example.com',
      },
      new Map([['https://api.example.com', 'intl']]),
    );
    expect(submission).toEqual({
      instanceName: 'main',
      apiKey: { api_key: 'new-key', group_id: 'g' },
      baseUrl: 'https://api.example.com',
      region: 'intl',
    });
  });

  it('detects typed credentials and keeps only safe settings', () => {
    expect(hasCredentialInput({ api_key: '', group_id: 'g' })).toBe(false);
    expect(hasCredentialInput({ api_key: 'k' })).toBe(true);
    expect(pickSafeSettings({ api_key: '', group_id: 'g' })).toEqual({
      group_id: 'g',
    });
    expect(pickSafeSettings('plain-key')).toBeUndefined();
    expect(
      pickSafeSettings({ mineru_server_url: '', mineru_backend: 'x' }),
    ).toEqual({
      mineru_backend: 'x',
    });
  });
});

describe('saved field checks', () => {
  const local: AdminService.PlatformInstanceDetail = {
    ...detail,
    api_key_config: {},
    credential_status: { api_key: true },
  };
  it('finds the field each provider saves as the instance base_url', () => {
    expect(baseUrlFieldOf(getProviderConfig('OpenAI'))).toBe('base_url');
    expect(baseUrlFieldOf(getProviderConfig('PaddleOCR.local'))).toBe(
      'paddleocr_api_url',
    );
    expect(baseUrlFieldOf(getProviderConfig('MinerU'))).toBe(
      'mineru_apiserver',
    );
    expect(baseUrlFieldOf(getProviderConfig('PaddleOCR'))).toBeUndefined();
  });
  it('finds a hidden address only where the provider stores it', () => {
    expect(
      isHiddenSavedAddress(local, 'paddleocr_api_url', 'paddleocr_api_url'),
    ).toBe(true);
    // Cloud PaddleOCR keeps its address in the configuration, not base_url.
    expect(isHiddenSavedAddress(local, 'paddleocr_api_url', undefined)).toBe(
      false,
    );
    expect(
      isHiddenSavedAddress(
        { ...local, api_key_config: { paddleocr_api_url: 'http://ocr' } },
        'paddleocr_api_url',
        'paddleocr_api_url',
      ),
    ).toBe(false);
  });
  it('asks again only for credentials and addresses saved with them', () => {
    // The server reuses a saved instance base_url when the test sends none.
    expect(
      missingTestFields(
        local,
        ['paddleocr_access_token', 'paddleocr_api_url'],
        { paddleocr_access_token: 'new' },
      ),
    ).toEqual([]);
    // Cloud PaddleOCR keeps a hidden address in the configuration.
    const cloud: AdminService.PlatformInstanceDetail = {
      ...detail,
      api_key_config: { paddleocr_api_url: '' },
      api_key_config_hidden: ['paddleocr_api_url'],
      credential_status: { paddleocr_access_token: true },
    };
    const fields = ['paddleocr_access_token', 'paddleocr_api_url'];
    expect(
      missingTestFields(cloud, fields, { paddleocr_access_token: 'new' }),
    ).toEqual(['paddleocr_api_url']);
    expect(
      missingTestFields(cloud, fields, {
        paddleocr_access_token: 'new',
        paddleocr_api_url: 'http://ocr',
      }),
    ).toEqual([]);
  });
  it('does not ask for a server URL that MinerU does not save', () => {
    // A pipeline backend has no server URL; its hidden endpoint is the
    // instance base_url.
    const mineru: AdminService.PlatformInstanceDetail = {
      ...detail,
      base_url: '',
      api_key_config: { mineru_backend: 'pipeline' },
      credential_status: { mineru_api_token: true },
    };
    const baseUrlField = baseUrlFieldOf(getProviderConfig('MinerU'));
    expect(isHiddenSavedAddress(mineru, 'mineru_apiserver', baseUrlField)).toBe(
      true,
    );
    expect(
      isHiddenSavedAddress(mineru, 'mineru_server_url', baseUrlField),
    ).toBe(false);
    expect(
      missingTestFields(
        mineru,
        ['mineru_apiserver', 'mineru_api_token', 'mineru_server_url'],
        { mineru_api_token: 'new' },
      ),
    ).toEqual([]);
  });
  it('treats an address saved empty as not configured', () => {
    const mineru: AdminService.PlatformInstanceDetail = {
      ...detail,
      base_url: 'http://mineru:8000',
      api_key_config: {
        mineru_backend: 'vlm-http-client',
        mineru_server_url: '',
      },
      api_key_config_hidden: [],
      credential_status: { mineru_api_token: true },
    };
    expect(
      isHiddenSavedAddress(mineru, 'mineru_server_url', 'mineru_apiserver'),
    ).toBe(false);
    expect(
      missingTestFields(mineru, ['mineru_api_token', 'mineru_server_url'], {
        mineru_api_token: 'new',
      }),
    ).toEqual([]);
  });
});

describe('cleared settings', () => {
  it('removes saved settings the admin emptied', () => {
    // MiniMax falls back to a plain key when group_id is empty.
    expect(
      withClearedSettings('', detail, ['api_key', 'group_id'], {
        api_key: '',
        group_id: '',
      }),
    ).toEqual({ group_id: null });
    expect(
      withClearedSettings('new-key', detail, ['api_key', 'group_id'], {
        api_key: 'new-key',
        group_id: '',
      }),
    ).toEqual({ api_key: 'new-key', group_id: null });
  });
  it('keeps blank credentials, hidden fields and unchanged settings', () => {
    const apiKey = { api_key: '', group_id: 'group-1' };
    expect(
      withClearedSettings(apiKey, detail, ['api_key', 'group_id'], apiKey),
    ).toBe(apiKey);
    // A field that is not shown is not cleared.
    expect(withClearedSettings('', detail, ['api_key'], { api_key: '' })).toBe(
      '',
    );
  });
});

describe('saved-credential test settings', () => {
  it('sends emptied settings as removed, like saving', () => {
    const cleared = withClearedSettings('', detail, ['api_key', 'group_id'], {
      api_key: '',
      group_id: '',
    });
    expect(pickSafeSettings(cleared)).toEqual({ group_id: null });
  });
});

describe('unsaved defaults', () => {
  const azure: AdminService.PlatformInstanceDetail = {
    ...detail,
    api_key_config: {},
  };
  it('leaves out a default the admin did not choose', () => {
    expect(
      withoutUnsavedDefaults(
        { api_key: '', api_version: '2024-10-21' },
        azure,
        { api_key: '', api_version: '2024-10-21' },
        { api_key: '', api_version: '2024-10-21' },
      ),
    ).toEqual({ api_key: '' });
  });
  it('keeps a value the admin typed or one already saved', () => {
    expect(
      withoutUnsavedDefaults(
        { api_key: '', api_version: '2025-01-01' },
        azure,
        { api_version: '2024-10-21' },
        { api_version: '2025-01-01' },
      ),
    ).toEqual({ api_key: '', api_version: '2025-01-01' });
    const saved = { ...azure, api_key_config: { api_version: '2024-10-21' } };
    expect(
      withoutUnsavedDefaults(
        { api_version: '2024-10-21' },
        saved,
        { api_version: '2024-10-21' },
        { api_version: '2024-10-21' },
      ),
    ).toEqual({ api_version: '2024-10-21' });
  });
  it('keeps a setting the admin changed back to the default', () => {
    expect(
      withoutUnsavedDefaults(
        { api_key: '', api_version: '2024-10-21' },
        azure,
        { api_version: '2024-10-21' },
        { api_version: '2024-10-21' },
        new Set(['api_version']),
      ),
    ).toEqual({ api_key: '', api_version: '2024-10-21' });
  });
});
