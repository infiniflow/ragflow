import {
  OntologyOverrideMode,
  ontologyBindingIssues,
  ontologyBindingPayload,
  ontologyOverrideMode,
} from './ontology-binding';

const t = (key: string) => key;

describe('ontologyOverrideMode', () => {
  it('reads a missing binding as inheriting the deployment default', () => {
    expect(ontologyOverrideMode(undefined)).toBe(OntologyOverrideMode.Inherit);
    expect(ontologyOverrideMode(null)).toBe(OntologyOverrideMode.Inherit);
  });

  it('reads an explicit disable as off', () => {
    expect(ontologyOverrideMode({ enabled: false })).toBe(
      OntologyOverrideMode.Off,
    );
  });

  it('reads an absent enabled as on, not off', () => {
    expect(ontologyOverrideMode({ model_id: 'tpl-1' })).toBe(
      OntologyOverrideMode.On,
    );
  });
});

describe('ontologyBindingPayload', () => {
  it('clears the stored setting when inheriting', () => {
    // null, not an omitted key: the server merges the patch into the stored
    // prompt_config, so an omitted key would leave the old override in place.
    expect(
      ontologyBindingPayload(OntologyOverrideMode.Inherit, {
        modelId: 'tpl-1',
      }),
    ).toBeNull();
  });

  it('writes only the disable flag when turned off', () => {
    expect(
      ontologyBindingPayload(OntologyOverrideMode.Off, {
        modelId: 'tpl-1',
        knowledgeDatasetIds: ['kb-1'],
      }),
    ).toEqual({ enabled: false });
  });

  it('writes both halves when turned on', () => {
    expect(
      ontologyBindingPayload(OntologyOverrideMode.On, {
        modelId: ' tpl-1 ',
        modelVersion: 2,
        knowledgeDatasetIds: ['kb-1'],
      }),
    ).toEqual({
      enabled: true,
      model_id: 'tpl-1',
      model_version: 2,
      knowledge_dataset_ids: ['kb-1'],
    });
  });

  it('falls back to the published version and an empty scope', () => {
    expect(ontologyBindingPayload(OntologyOverrideMode.On, {})).toEqual({
      enabled: true,
      model_id: '',
      model_version: 0,
      knowledge_dataset_ids: [],
    });
  });
});

describe('ontologyBindingIssues', () => {
  it('accepts an absent binding', () => {
    expect(ontologyBindingIssues({ prompt_config: {} }, t)).toEqual([]);
  });

  it('accepts an explicit disable without a model or a scope', () => {
    expect(
      ontologyBindingIssues(
        { prompt_config: { ontology: { enabled: false } } },
        t,
      ),
    ).toEqual([]);
  });

  it('accepts a complete binding', () => {
    expect(
      ontologyBindingIssues(
        {
          prompt_config: {
            ontology: {
              enabled: true,
              model_id: 'tpl-1',
              knowledge_dataset_ids: ['kb-1'],
            },
          },
        },
        t,
      ),
    ).toEqual([]);
  });

  it('flags a binding with no model', () => {
    expect(
      ontologyBindingIssues(
        {
          prompt_config: {
            ontology: { enabled: true, knowledge_dataset_ids: ['kb-1'] },
          },
        },
        t,
      ),
    ).toContainEqual({
      path: ['prompt_config', 'ontology', 'model_id'],
      message: 'ontologyModelRequired',
    });
  });

  it('flags a binding with no knowledge scope', () => {
    expect(
      ontologyBindingIssues(
        { prompt_config: { ontology: { enabled: true, model_id: 'tpl-1' } } },
        t,
      ),
    ).toContainEqual({
      path: ['prompt_config', 'ontology', 'knowledge_dataset_ids'],
      message: 'ontologyKnowledgeScopeRequired',
    });
  });

  it('flags both halves when neither was chosen', () => {
    expect(
      ontologyBindingIssues({ prompt_config: { ontology: {} } }, t),
    ).toHaveLength(2);
  });

  it('treats a whitespace-only model id as missing', () => {
    expect(
      ontologyBindingIssues(
        {
          prompt_config: {
            ontology: { model_id: '   ', knowledge_dataset_ids: ['kb-1'] },
          },
        },
        t,
      ),
    ).toContainEqual({
      path: ['prompt_config', 'ontology', 'model_id'],
      message: 'ontologyModelRequired',
    });
  });
});
