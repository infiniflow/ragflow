import {
  approveEvaDocumentChange,
  assignBusinessDocumentOwner,
  BusinessDocumentConflictError,
  createBusinessDocument,
  createEvaChangeFromBusinessDocument,
  createEvaDocumentChange,
  deleteBusinessDocument,
  downloadBusinessDocumentExport,
  fetchBusinessDocument,
  fetchEvaDocumentChange,
  generateEvaDocumentChangeDraft,
  listBusinessDocumentAccessUsers,
  listBusinessDocumentRevisions,
  listBusinessDocuments,
  listEvaDocumentChanges,
  prepareEvaDocumentChange,
  publishEvaDocumentChange,
  pullBusinessDocumentFromEva,
  rebindBusinessDocumentToEva,
  searchEvaDocumentSources,
  submitBusinessDocumentCommand,
} from '@/services/business-document-service';
import { listEvaUserCredentials } from '@/services/user-service';
import { downloadFileFromBlob } from '@/utils/file-util';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { MemoryRouter, Route, Routes as RouterRoutes } from 'react-router';
import BusinessDocumentsPage from '.';

type BusinessDocumentProjection = import('./types').BusinessDocumentProjection;
type BusinessDocumentCommandResult =
  import('./types').BusinessDocumentCommandResult;
type EvaDocumentChange = import('./types').EvaDocumentChange;

jest.mock('react-markdown', () => ({
  __esModule: true,
  default: ({ children }: { children?: string }) => children ?? null,
}));

jest.mock('remark-gfm', () => ({
  __esModule: true,
  default: () => undefined,
}));

jest.mock('@/components/ui/audio-button', () => ({
  AudioButton: ({
    ariaLabel,
    disabled,
    onOk,
    testId,
  }: {
    ariaLabel?: string;
    disabled?: boolean;
    onOk?: (transcript: string) => void;
    testId?: string;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      data-testid={testId}
      disabled={disabled}
      onClick={() => onOk?.('текст из диктовки')}
    >
      Голосовой ввод
    </button>
  ),
}));

jest.mock('@/utils/file-util', () => ({
  downloadFileFromBlob: jest.fn(),
}));

jest.mock('@/services/business-document-service', () => {
  const actual = jest.requireActual('@/services/business-document-service');
  return {
    ...actual,
    createBusinessDocument: jest.fn(),
    createEvaChangeFromBusinessDocument: jest.fn(),
    deleteBusinessDocument: jest.fn(),
    downloadBusinessDocumentExport: jest.fn(),
    fetchBusinessDocument: jest.fn(),
    listBusinessDocuments: jest.fn(),
    listBusinessDocumentRevisions: jest.fn(),
    listBusinessDocumentAccessUsers: jest.fn(),
    assignBusinessDocumentOwner: jest.fn(),
    submitBusinessDocumentCommand: jest.fn(),
    searchEvaDocumentSources: jest.fn(),
    createEvaDocumentChange: jest.fn(),
    listEvaDocumentChanges: jest.fn(),
    fetchEvaDocumentChange: jest.fn(),
    generateEvaDocumentChangeDraft: jest.fn(),
    approveEvaDocumentChange: jest.fn(),
    prepareEvaDocumentChange: jest.fn(),
    publishEvaDocumentChange: jest.fn(),
    pullBusinessDocumentFromEva: jest.fn(),
    rebindBusinessDocumentToEva: jest.fn(),
  };
});

jest.mock('@/services/user-service', () => ({
  listEvaUserCredentials: jest.fn(),
}));

const mockedCreate = jest.mocked(createBusinessDocument);
const mockedCreateEvaFromDocument = jest.mocked(
  createEvaChangeFromBusinessDocument,
);
const mockedDelete = jest.mocked(deleteBusinessDocument);
const mockedDownloadExport = jest.mocked(downloadBusinessDocumentExport);
const mockedDownloadFileFromBlob = jest.mocked(downloadFileFromBlob);
const mockedFetch = jest.mocked(fetchBusinessDocument);
const mockedList = jest.mocked(listBusinessDocuments);
const mockedListRevisions = jest.mocked(listBusinessDocumentRevisions);
const mockedListAccessUsers = jest.mocked(listBusinessDocumentAccessUsers);
const mockedAssignOwner = jest.mocked(assignBusinessDocumentOwner);
const mockedSubmit = jest.mocked(submitBusinessDocumentCommand);
const mockedSearchEva = jest.mocked(searchEvaDocumentSources);
const mockedCreateEva = jest.mocked(createEvaDocumentChange);
const mockedListEva = jest.mocked(listEvaDocumentChanges);
const mockedFetchEva = jest.mocked(fetchEvaDocumentChange);
const mockedGenerateEva = jest.mocked(generateEvaDocumentChangeDraft);
const mockedApproveEva = jest.mocked(approveEvaDocumentChange);
const mockedPrepareEva = jest.mocked(prepareEvaDocumentChange);
const mockedPublishEva = jest.mocked(publishEvaDocumentChange);
const mockedPullEvaDocument = jest.mocked(pullBusinessDocumentFromEva);
const mockedRebindEvaDocument = jest.mocked(rebindBusinessDocumentToEva);
const mockedListEvaUserCredentials = jest.mocked(listEvaUserCredentials);

const firstSectionText =
  'Повторяемая фраза. Сократить время перевода. Результат измеряется в минутах.';
const secondSectionText =
  'Повторяемая фраза. Текущий процесс состоит из нескольких шагов.';

const projection: BusinessDocumentProjection = {
  document_id: 'doc-1',
  title: 'Переводы одной кнопкой',
  document_type: 'business_requirements',
  state_version: 18,
  lifecycle_state: 'REVIEW',
  operation_state: 'IDLE',
  current_revision: {
    revision_id: 'revision-3',
    revision_number: 3,
    document_ast: {
      schema_version: '1',
      document_type: 'business_requirements',
      template_version: '1.0.0',
      sections: [
        {
          id: '1',
          title: 'Цель',
          blocks: [{ type: 'paragraph', text: firstSectionText }],
        },
        {
          id: '2',
          title: 'Текущий процесс',
          blocks: [{ type: 'paragraph', text: secondSectionText }],
        },
        {
          id: '3',
          title: 'Таблица значений',
          blocks: [
            {
              type: 'table',
              headers: ['Флаг', 'Значение'],
              rows: [
                [true, null],
                [false, 7],
              ],
            },
          ],
        },
      ],
    },
    section_texts: {
      '1': firstSectionText,
      '2': secondSectionText,
      '3': '| Флаг | Значение |\n| --- | --- |\n| True | None |\n| False | 7 |',
    },
    body_markdown: `## 1. Цель\n${firstSectionText}\n\n## 2. Текущий процесс\n${secondSectionText}`,
    content_hash: 'sha256:body',
  },
  active_review_cycle: 2,
  protocol: {
    questions: [
      {
        question_id: 'question-9',
        sequence_number: 9,
        target_section_id: '5.5',
        text: 'Какие события необходимо отслеживать?',
        options: [
          { option_id: 'success', label: 'Только успешные операции' },
          {
            option_id: 'all',
            label: 'Успешные и неуспешные операции',
          },
        ],
        allow_custom_answer: true,
        status: 'OPEN',
      },
    ],
    proposals: [
      {
        proposal_id: 'proposal-12',
        target_section_id: '3.1',
        text: 'Добавить количественную оценку нагрузки',
        rationale: 'Она нужна для нефункциональных требований',
        decision: 'PENDING',
      },
    ],
    comments: [
      {
        comment_id: 'comment-4',
        section_id: '1',
        text: 'Уточнить терминологию',
        anchor_status: 'ANCHORED',
        anchor: {
          revision_id: 'revision-3',
          section_id: '1',
          selected_text: 'Сократить время перевода',
          prefix: 'Повторяемая фраза. ',
          suffix: '. Результат измеряется в минутах.',
          start_offset: firstSectionText.indexOf('Сократить время перевода'),
          end_offset:
            firstSectionText.indexOf('Сократить время перевода') +
            'Сократить время перевода'.length,
        },
        disposition: {
          comment_event_id: 'event-comment-4',
          disposition: 'CONFIRMED_CHANGE',
        },
      },
    ],
  },
  allowed_commands: [
    'ANSWER_QUESTION',
    'DECIDE_PROPOSAL',
    'ADD_COMMENT',
    'APPLY_CHANGES',
  ],
};

const commandResult: BusinessDocumentCommandResult = {
  accepted: true,
  document_id: 'doc-1',
  state_version: 19,
  lifecycle_state: 'REVIEW',
  operation_state: 'ANALYZING',
  job_id: 'job-1',
};

const evaChange: EvaDocumentChange = {
  change_id: 'change-1',
  state_version: 2,
  workflow_state: 'EDITING',
  change_summary: 'Уточнить ожидаемый результат.',
  source: {
    connector_id: 'connector-1',
    project_id: 'project-1',
    document_id: 'CmfDocument:doc-1',
    document_code: 'BR-42',
    document_name: 'Переводы одной кнопкой',
    web_url: 'https://eva.example.com/project/Document/BR-42',
    base_version: '1|version-1|2026-08-26',
    base_content_hash: 'sha256:base',
  },
  base_markdown: '# Требования\n\n## Цель\n\nСтарый текст.',
  draft_markdown: '# Требования\n\n## Цель\n\nНовый текст.',
  draft_content_hash: 'sha256:draft',
  diff: {
    changed: true,
    added_lines: 1,
    removed_lines: 1,
    changed_sections: 1,
    sections: [
      {
        key: 'цель:1',
        title: 'Цель',
        lines: [
          { type: 'context', content: '## Цель' },
          { type: 'removed', content: 'Старый текст.' },
          { type: 'added', content: 'Новый текст.' },
        ],
      },
    ],
  },
  allowed_actions: ['GENERATE_DRAFT', 'APPROVE'],
  events: [
    {
      event_id: 'event-1',
      sequence: 1,
      event_type: 'CHANGE_REQUEST_CREATED',
      actor_id: 'author-1',
      payload: {},
      create_time: 1,
    },
  ],
};

function renderPage(path = '/business-documents/doc-1') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <RouterRoutes>
          <Route
            path="/business-documents"
            element={<BusinessDocumentsPage />}
          />
          <Route
            path="/business-documents/:id"
            element={<BusinessDocumentsPage />}
          />
          <Route
            path="/business-documents/eva/:changeId"
            element={<BusinessDocumentsPage />}
          />
        </RouterRoutes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function mockTextSelection(
  selectedText: string,
  startSection: HTMLElement,
  endSection: HTMLElement = startSection,
) {
  const startNode = startSection.firstChild ?? startSection;
  const endNode = endSection.firstChild ?? endSection;
  const commonAncestorContainer =
    startSection === endSection
      ? startSection
      : screen.getByTestId('business-document-markdown');
  return jest.spyOn(window, 'getSelection').mockReturnValue({
    rangeCount: 1,
    getRangeAt: () =>
      ({
        commonAncestorContainer,
        startContainer: startNode,
        endContainer: endNode,
      }) as unknown as Range,
    toString: () => selectedText,
  } as unknown as Selection);
}

beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn();
  global.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

beforeEach(() => {
  jest.clearAllMocks();
  mockedFetch.mockResolvedValue(projection);
  mockedList.mockResolvedValue({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
  });
  mockedListRevisions.mockResolvedValue([projection.current_revision!]);
  mockedListAccessUsers.mockResolvedValue({ items: [] });
  mockedAssignOwner.mockResolvedValue(projection);
  mockedListEva.mockResolvedValue({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
  });
  mockedFetchEva.mockResolvedValue(evaChange);
  mockedGenerateEva.mockResolvedValue(evaChange);
  mockedApproveEva.mockResolvedValue(evaChange);
  mockedPrepareEva.mockResolvedValue(evaChange);
  mockedPublishEva.mockResolvedValue(evaChange);
  mockedSubmit.mockResolvedValue(commandResult);
  mockedCreate.mockResolvedValue(projection);
  mockedDelete.mockResolvedValue({
    document_id: 'saved-1',
    deleted: true,
    deleted_artifacts: 0,
    storage_cleanup_failures: 0,
  });
  mockedCreateEvaFromDocument.mockResolvedValue(evaChange);
  mockedPullEvaDocument.mockResolvedValue({
    document: projection,
    sync: { changed: false, direction: 'FROM_EVA' },
  });
  mockedListEvaUserCredentials.mockResolvedValue({
    data: {
      code: 0,
      data: {
        items: [
          {
            scope: 'https://eva.example.com',
            configured: true,
            connector_id: 'connector-1',
            connectors: [{ id: 'connector-1', name: 'EVA Wiki' }],
          },
        ],
      },
    },
  });
});

test('renders a dense read-only workbench from the server projection', async () => {
  renderPage();

  expect(
    await screen.findByTestId('business-document-workbench'),
  ).toBeInTheDocument();
  expect(screen.getByTestId('business-document-pane')).toHaveTextContent(
    'Сократить время перевода',
  );
  expect(screen.getByTestId('business-document-protocol')).toHaveTextContent(
    'Какие события необходимо отслеживать?',
  );
  expect(screen.getByTestId('business-document-question')).toBeInTheDocument();
  expect(screen.getByTestId('business-document-proposal')).toBeInTheDocument();
  expect(screen.getByTestId('business-document-comment')).toBeInTheDocument();
  expect(
    screen.getByTestId('business-document-comment-disposition'),
  ).toHaveTextContent('Подтверждено к правке');
  expect(screen.getByTestId('apply-changes-button')).toHaveTextContent(
    'Применить исправления',
  );
  expect(screen.queryByRole('textbox', { name: 'Документ' })).toBeNull();
  expect(screen.getAllByTestId('business-document-section')[2]).toHaveAttribute(
    'data-section-text',
    '| Флаг | Значение |\n| --- | --- |\n| True | None |\n| False | 7 |',
  );
});

test('shows an explicit message for an empty document section', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    current_revision: {
      ...projection.current_revision!,
      document_ast: {
        ...projection.current_revision!.document_ast,
        sections: [
          ...projection.current_revision!.document_ast.sections,
          { id: '4', title: 'Ограничения', blocks: [] },
        ],
      },
      section_texts: {
        ...projection.current_revision!.section_texts,
        '4': '',
      },
    },
  });
  renderPage();

  const sections = await screen.findAllByTestId('business-document-section');
  expect(sections[3]).toHaveTextContent('4. Ограничения');
  expect(sections[3]).toHaveTextContent('Требования отсутствуют');
  expect(sections[3]).toHaveAttribute('data-section-text', '');
});

test('uses semantic status colors for questions and proposals', async () => {
  mockedFetch.mockResolvedValue({
    ...projection,
    protocol: {
      ...projection.protocol,
      questions: [
        ...projection.protocol.questions,
        {
          ...projection.protocol.questions[0],
          question_id: 'question-answered',
          sequence_number: 10,
          status: 'ANSWERED',
          answer: { selected_option_id: 'all' },
        },
        {
          ...projection.protocol.questions[0],
          question_id: 'question-cancelled',
          sequence_number: 11,
          status: 'CANCELLED',
        },
      ],
      proposals: [
        ...projection.protocol.proposals,
        {
          ...projection.protocol.proposals[0],
          proposal_id: 'proposal-accepted',
          decision: 'ACCEPTED',
        },
        {
          ...projection.protocol.proposals[0],
          proposal_id: 'proposal-rejected',
          decision: 'REJECTED',
        },
      ],
    },
  });
  renderPage();

  const questions = await screen.findAllByTestId('business-document-question');
  const progress = screen.getByTestId('business-document-review-progress');
  expect(progress).toHaveTextContent('Ответы1/2');
  expect(progress).toHaveTextContent('Предложения2/3');
  expect(progress).toHaveTextContent('Закрыто: 1');
  expect(
    screen.getByRole('progressbar', { name: 'Ответы на вопросы' }),
  ).toHaveAttribute('aria-valuemax', '2');
  expect(
    screen.getByRole('progressbar', { name: 'Решения по предложениям' }),
  ).toHaveAttribute('aria-valuenow', '2');
  expect(questions.find((item) => item.dataset.status === 'OPEN')).toHaveClass(
    'bg-accent-primary/5',
  );
  expect(
    questions.find((item) => item.dataset.status === 'ANSWERED'),
  ).toHaveClass('bg-state-success/5');
  expect(
    questions.find((item) => item.dataset.status === 'CANCELLED'),
  ).toHaveClass('bg-bg-card/40');
  expect(screen.getByTestId('question-status-question-9')).toHaveTextContent(
    'Требует ответа',
  );
  expect(screen.getByTestId('question-status-question-answered')).toHaveClass(
    'text-state-success',
  );
  expect(
    screen.getByTestId('question-status-question-cancelled'),
  ).toHaveTextContent('Закрыт');

  const proposals = screen.getAllByTestId('business-document-proposal');
  expect(
    proposals.find((item) => item.dataset.status === 'PENDING'),
  ).toHaveClass('bg-state-warning/5');
  expect(
    proposals.find((item) => item.dataset.status === 'ACCEPTED'),
  ).toHaveClass('bg-state-success/5');
  expect(
    proposals.find((item) => item.dataset.status === 'REJECTED'),
  ).toHaveClass('bg-state-error/5');
  expect(
    screen.getByTestId('proposal-status-proposal-accepted'),
  ).toHaveTextContent('Принято');
  expect(screen.getByTestId('proposal-status-proposal-rejected')).toHaveClass(
    'text-state-error',
  );
});

test('renders an undecided proposal as closed history after review completion', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'AGREED',
    allowed_commands: ['START_REVIEW', 'REQUEST_EXPORT', 'ARCHIVE'],
  });
  renderPage();

  const status = await screen.findByTestId('proposal-status-proposal-12');
  expect(
    screen.getByTestId('business-document-review-progress'),
  ).toHaveTextContent('Без решения: 1');
  expect(
    screen.getByRole('progressbar', { name: 'Решения по предложениям' }),
  ).toHaveAttribute('aria-valuenow', '0');
  expect(status).toHaveTextContent('Не принято');
  expect(status).toHaveClass('text-text-disabled');
  expect(screen.getByTestId('closed-proposal-proposal-12')).toHaveTextContent(
    'Решение не было принято до завершения этапа внесения изменений.',
  );
  expect(
    screen.queryByTestId('accept-proposal-proposal-12'),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByTestId('reject-proposal-proposal-12'),
  ).not.toBeInTheDocument();
});

test('shows which comments, questions and AI proposals produced each revision', async () => {
  mockedListRevisions.mockResolvedValueOnce([
    {
      ...projection.current_revision!,
      author_id: 'author-4',
      author_name: 'Мария Авторова',
      author_login: 'maria@example.test',
      created_at: 1_788_200_000,
      change_basis: [
        {
          event_id: 'event-draft',
          actor_id: 'ai-worker-1',
          actor_type: 'AI',
          initiated_by_actor_id: 'author-4',
          initiated_by_actor_name: 'Мария Авторова',
          initiated_by_actor_login: 'maria@example.test',
          type: 'INITIAL_DRAFT',
          title: 'Первичный черновик',
          summary: 'Исходная идея документа.',
        },
        {
          event_id: 'event-question',
          actor_id: 'author-2',
          actor_type: 'USER',
          actor_name: 'Иван Редактор',
          actor_login: 'ivan@example.test',
          type: 'QUESTION',
          title: 'Ответ на вопрос',
          summary: 'Как измеряется скорость перевода?',
          details: 'Не более двух минут',
          section_id: '3.1',
        },
        {
          event_id: 'event-comment',
          actor_id: 'author-3',
          actor_type: 'USER',
          actor_name: 'Анна Эксперт',
          actor_login: 'anna@example.test',
          type: 'COMMENT',
          title: 'Комментарий автора',
          summary: 'Добавить негативный сценарий.',
          section_id: '4.3',
        },
        {
          event_id: 'event-proposal',
          type: 'PROPOSAL',
          title: 'Принятое предложение ИИ',
          summary: 'Уточнить критерий успешности.',
          section_id: '5.5',
        },
      ],
    },
  ]);
  renderPage();

  fireEvent.click(
    await screen.findByTestId('business-document-history-toggle'),
  );

  expect(
    await screen.findByText('Как измеряется скорость перевода?'),
  ).toBeVisible();
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Добавить негативный сценарий.',
  );
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Принятое предложение ИИ',
  );
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Изменил: Иван Редактор (ivan@example.test)',
  );
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Изменил: Анна Эксперт (anna@example.test)',
  );
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Инициировал: Мария Авторова (maria@example.test)',
  );
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Исполнитель: ai-worker-1',
  );
  expect(screen.getByTestId('business-document-history')).toHaveTextContent(
    'Автор изменений: Мария Авторова (maria@example.test)',
  );
  expect(mockedListRevisions).toHaveBeenCalledWith('doc-1');
});

test('offers personal-token EVA actions for a verified page binding', async () => {
  const linked = {
    ...projection,
    lifecycle_state: 'AGREED' as const,
    operation_state: 'IDLE' as const,
    eva_binding: {
      page_url: 'https://eva.example.com/project/Document/BR-42',
      status: 'CONNECTED' as const,
      capabilities: [
        'OPEN' as const,
        'PULL_FROM_EVA' as const,
        'CREATE_EVA_CHANGE' as const,
      ],
      connector_id: 'connector-1',
      document_id: 'eva-document-1',
      document_code: 'BR-42',
      document_name: 'Переводы одной кнопкой',
    },
  };
  mockedFetch.mockResolvedValueOnce(linked);
  mockedPullEvaDocument.mockResolvedValueOnce({
    document: { ...linked, lifecycle_state: 'REVIEW' },
    sync: { changed: true, direction: 'FROM_EVA', event_id: 'eva-pull-1' },
  });
  renderPage();

  expect(
    await screen.findByTestId('business-document-eva-binding'),
  ).toHaveTextContent('Переводы одной кнопкой');
  expect(
    await screen.findByRole('button', {
      name: 'Сохранить текущий вариант в EVA',
    }),
  ).toBeVisible();
  expect(
    screen.getByRole('button', {
      name: 'Перечитать текущий документ из EVA',
    }),
  ).toBeVisible();
  fireEvent.click(screen.getByTestId('pull-business-document-from-eva'));
  await waitFor(() =>
    expect(mockedPullEvaDocument).toHaveBeenCalledWith('doc-1', 18),
  );
  expect(
    await screen.findByText(
      'Новая версия EVA добавлена в цикл внесения изменений. Запустите анализ замечаний.',
    ),
  ).toBeVisible();
});

test('hides document EVA actions when the personal token is absent', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'AGREED',
    eva_binding: {
      page_url: 'https://eva.example.com/project/Document/BR-42',
      status: 'CONNECTED',
      capabilities: ['OPEN', 'PULL_FROM_EVA', 'CREATE_EVA_CHANGE'],
      connector_id: 'connector-1',
      document_id: 'eva-document-1',
    },
  });
  mockedListEvaUserCredentials.mockResolvedValueOnce({
    data: { code: 0, data: { items: [] } },
  });

  renderPage();

  expect(
    await screen.findByTestId('business-document-eva-binding'),
  ).toBeVisible();
  await waitFor(() => expect(mockedListEvaUserCredentials).toHaveBeenCalled());
  expect(
    screen.queryByRole('button', {
      name: 'Сохранить текущий вариант в EVA',
    }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole('button', {
      name: 'Перечитать текущий документ из EVA',
    }),
  ).not.toBeInTheDocument();
});

test('reconnects a linked EVA page before rereading it with a personal token', async () => {
  const linkOnly = {
    ...projection,
    lifecycle_state: 'AGREED' as const,
    eva_binding: {
      page_url: 'https://eva.example.com/project/Document/BR-42',
      status: 'LINK_ONLY' as const,
      capabilities: ['OPEN' as const],
      document_code: 'BR-42',
    },
  };
  const connected = {
    ...linkOnly,
    state_version: 19,
    eva_binding: {
      ...linkOnly.eva_binding,
      status: 'CONNECTED' as const,
      capabilities: [
        'OPEN' as const,
        'PULL_FROM_EVA' as const,
        'CREATE_EVA_CHANGE' as const,
      ],
      connector_id: 'connector-1',
      document_id: 'eva-document-1',
    },
  };
  mockedFetch.mockResolvedValueOnce(linkOnly);
  mockedRebindEvaDocument.mockResolvedValueOnce(connected);
  mockedPullEvaDocument.mockResolvedValueOnce({
    document: connected,
    sync: { changed: false, direction: 'FROM_EVA' },
  });

  renderPage();

  fireEvent.click(
    await screen.findByRole('button', {
      name: 'Перечитать текущий документ из EVA',
    }),
  );
  await waitFor(() =>
    expect(mockedRebindEvaDocument).toHaveBeenCalledWith('doc-1', 18),
  );
  await waitFor(() =>
    expect(mockedPullEvaDocument).toHaveBeenCalledWith('doc-1', 19),
  );
});

test('reconnects a link-only EVA page after a connector becomes available', async () => {
  const linkOnly = {
    ...projection,
    eva_binding: {
      page_url: 'http://host.docker.internal:8084//project/Document/DOC-001883',
      status: 'LINK_ONLY' as const,
      capabilities: ['OPEN' as const],
      document_code: 'DOC-001883',
    },
  };
  const connected = {
    ...linkOnly,
    state_version: 19,
    eva_binding: {
      page_url: 'https://eva.example.com/project/Document/DOC-001883',
      status: 'CONNECTED' as const,
      capabilities: [
        'OPEN' as const,
        'PULL_FROM_EVA' as const,
        'CREATE_EVA_CHANGE' as const,
      ],
      connector_id: 'connector-business-documents',
      document_id: 'CmfDocument:doc-1',
      document_code: 'DOC-001883',
      document_name: 'Документ1',
    },
  };
  mockedFetch.mockResolvedValueOnce(linkOnly);
  mockedRebindEvaDocument.mockResolvedValueOnce(connected);
  renderPage();

  expect(
    await screen.findByText('Только ссылка — доступный коннектор не найден'),
  ).toBeVisible();
  fireEvent.click(screen.getByTestId('rebind-business-document-to-eva'));

  await waitFor(() =>
    expect(mockedRebindEvaDocument).toHaveBeenCalledWith('doc-1', 18),
  );
  expect(
    await screen.findByText('Страница EVA подключена. Синхронизация доступна.'),
  ).toBeVisible();
  expect(screen.getByTestId('business-document-eva-binding')).toHaveTextContent(
    'Документ1',
  );
});

test('keeps EVA pull disabled until the first local revision exists', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'INTAKE',
    current_revision: null,
    eva_binding: {
      page_url: 'https://eva.example.com/project/Document/BR-42',
      status: 'CONNECTED',
      capabilities: ['OPEN', 'PULL_FROM_EVA'],
      connector_id: 'connector-1',
      document_id: 'eva-document-1',
    },
  });
  renderPage();

  const pullButton = await screen.findByTestId(
    'pull-business-document-from-eva',
  );
  expect(pullButton).toBeDisabled();
  fireEvent.click(pullButton);
  expect(mockedPullEvaDocument).not.toHaveBeenCalled();
});

test('opens the guarded EVA publication workflow from an agreed revision', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'AGREED',
    operation_state: 'IDLE',
    eva_binding: {
      page_url: 'https://eva.example.com/project/Document/BR-42',
      status: 'CONNECTED',
      capabilities: ['OPEN', 'CREATE_EVA_CHANGE'],
      connector_id: 'connector-1',
      document_id: 'eva-document-1',
    },
  });
  renderPage();

  fireEvent.click(await screen.findByTestId('push-business-document-to-eva'));
  await waitFor(() =>
    expect(mockedCreateEvaFromDocument).toHaveBeenCalledWith('doc-1', 18),
  );
  expect(await screen.findByTestId('eva-change-workbench')).toBeVisible();
});

test('marks a preserved comment when its anchor belongs to an older revision', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    protocol: {
      ...projection.protocol,
      comments: projection.protocol.comments.map((comment) => ({
        ...comment,
        anchor_status: 'ORPHANED' as const,
        revision_id: 'revision-2',
      })),
    },
  });
  renderPage();

  expect(
    await screen.findByTestId('business-document-orphaned-anchor'),
  ).toHaveTextContent('Фрагмент относится к предыдущей ревизии');
  expect(screen.getByTestId('business-document-comment')).toHaveTextContent(
    'Уточнить терминологию',
  );
});

test('shows all agent comment dispositions as read-only protocol status', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    protocol: {
      ...projection.protocol,
      comments: [
        {
          ...projection.protocol.comments[0],
          comment_id: 'comment-needs-question',
          disposition: {
            comment_event_id: 'event-needs-question',
            disposition: 'NEEDS_QUESTION',
            question_id: 'question-clarification',
            question_semantic_tag: 'scope.clarification',
          },
        },
        {
          ...projection.protocol.comments[0],
          comment_id: 'comment-no-change',
          disposition: {
            comment_event_id: 'event-no-change',
            disposition: 'NO_CHANGE',
          },
        },
      ],
    },
  });
  renderPage();

  const statuses = await screen.findAllByTestId(
    'business-document-comment-disposition',
  );
  expect(statuses.map((status) => status.textContent)).toEqual([
    'Требует уточнения',
    'Без изменения',
  ]);
  statuses.forEach((status) =>
    expect(status.querySelector('button')).toBeNull(),
  );
});

test('downloads completed exports through the authenticated request client', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    latest_exports: [
      {
        artifact_id: 'artifact-md-r2',
        revision_id: 'revision-2',
        revision_number: 2,
        format: 'MARKDOWN',
        filename: 'requirements_r2.md',
        mime_type: 'text/markdown; charset=utf-8',
        size: 512,
        content_hash: 'sha256:artifact',
        create_time: 1_787_695_200,
      },
    ],
  });
  renderPage();

  const exports = await screen.findByTestId('business-document-exports');
  const download = screen.getByRole('link', { name: /Markdown\s+r2/ });
  expect(exports).toContainElement(download);
  expect(download).toHaveAttribute(
    'href',
    '/api/v1/business-documents/doc-1/exports/artifact-md-r2/download',
  );
  expect(download).toHaveAttribute('download', 'requirements_r2.md');
  expect(download).not.toHaveTextContent('r3');

  const blob = new Blob(['# requirements'], { type: 'text/markdown' });
  mockedDownloadExport.mockResolvedValueOnce(blob);
  fireEvent.click(download);
  await waitFor(() =>
    expect(mockedDownloadExport).toHaveBeenCalledWith(
      'doc-1',
      'artifact-md-r2',
    ),
  );
  expect(mockedDownloadFileFromBlob).toHaveBeenCalledWith(
    blob,
    'requirements_r2.md',
  );
});

test('does not expose DOCX export actions or completed DOCX artifacts', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'AGREED',
    allowed_commands: ['REQUEST_EXPORT'],
    latest_exports: [
      {
        artifact_id: 'artifact-docx-r3',
        revision_id: 'revision-3',
        revision_number: 3,
        format: 'DOCX',
        filename: 'requirements_r3.docx',
        mime_type:
          'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        size: 1024,
        content_hash: 'sha256:docx-artifact',
        create_time: 1_787_695_200,
      },
    ],
  });
  renderPage();

  expect(
    await screen.findByRole('button', { name: 'Создать Markdown' }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole('button', { name: 'Создать HTML для EvaWiki' }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole('button', { name: 'DOCX' }),
  ).not.toBeInTheDocument();
  expect(screen.queryByText('Готовые файлы:')).not.toBeInTheDocument();
  expect(screen.queryByRole('link', { name: /Word/ })).not.toBeInTheDocument();
});

test('keeps all agreed-state actions in a wrapping mobile header', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'AGREED',
    allowed_commands: ['START_REVIEW', 'REQUEST_EXPORT', 'ARCHIVE'],
  });
  renderPage();

  const workbench = await screen.findByTestId('business-document-workbench');
  const actions = screen.getByTestId('business-document-actions');
  const title = screen.getByRole('heading', {
    name: 'Переводы одной кнопкой',
  });

  expect(workbench).toHaveClass('min-w-0');
  expect(actions).toHaveClass('w-full', 'min-w-0', 'flex-wrap');
  expect(actions).toHaveClass('sm:w-auto');
  expect(title).toHaveClass('break-words', 'sm:truncate');
  expect(
    screen.getByRole('button', { name: 'Начать вносить изменения' }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole('button', { name: 'Создать Markdown' }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole('button', { name: 'Создать HTML для EvaWiki' }),
  ).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'В архив' })).toBeInTheDocument();
});

test('gates every interaction using allowed_commands', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    allowed_commands: [],
  });
  renderPage();

  expect(
    await screen.findByTestId('answer-question-question-9'),
  ).toBeDisabled();
  expect(screen.getByTestId('accept-proposal-proposal-12')).toBeDisabled();
  expect(screen.getByTestId('reject-proposal-proposal-12')).toBeDisabled();
  expect(screen.getByRole('textbox', { name: 'Комментарий' })).toBeDisabled();
  expect(screen.queryByTestId('apply-changes-button')).toBeNull();
});

test('submits a structured answer and current state version', async () => {
  renderPage();

  fireEvent.click(
    await screen.findByRole('radio', {
      name: 'Успешные и неуспешные операции',
    }),
  );
  fireEvent.click(screen.getByTestId('answer-question-question-9'));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenCalledWith(
    'doc-1',
    expect.objectContaining({
      schema_version: '1',
      expected_state_version: 18,
      type: 'ANSWER_QUESTION',
      payload: {
        question_id: 'question-9',
        selected_option_id: 'all',
        custom_answer: null,
      },
    }),
  );
});

test('lets the author collapse and reopen an individual question', async () => {
  renderPage();

  const toggle = await screen.findByTestId('toggle-question-question-9');
  expect(toggle).toHaveAttribute('aria-expanded', 'true');

  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute('aria-expanded', 'false');

  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute('aria-expanded', 'true');
});

test('collapses an answered question and focuses the next unanswered one', async () => {
  mockedFetch.mockResolvedValue({
    ...projection,
    protocol: {
      ...projection.protocol,
      questions: [
        ...projection.protocol.questions,
        {
          question_id: 'question-10',
          sequence_number: 10,
          target_section_id: '5.6',
          text: 'Как долго хранить историю операций?',
          options: [
            { option_id: 'month', label: 'Один месяц' },
            { option_id: 'year', label: 'Один год' },
          ],
          allow_custom_answer: false,
          status: 'OPEN',
        },
      ],
    },
  });
  renderPage();

  const currentToggle = await screen.findByTestId('toggle-question-question-9');
  const nextToggle = screen.getByTestId('toggle-question-question-10');
  const scrollContainer = screen.getByTestId(
    'business-document-protocol-scroll',
  );
  const scrollIntoView = jest.fn();
  nextToggle.scrollIntoView = scrollIntoView;
  jest.spyOn(nextToggle, 'getBoundingClientRect').mockReturnValue({
    bottom: 540,
    height: 40,
    left: 0,
    right: 100,
    top: 500,
    width: 100,
    x: 0,
    y: 500,
    toJSON: () => ({}),
  });
  jest.spyOn(scrollContainer, 'getBoundingClientRect').mockReturnValue({
    bottom: 300,
    height: 300,
    left: 0,
    right: 430,
    top: 0,
    width: 430,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  });

  expect(currentToggle).toHaveAttribute('aria-expanded', 'true');
  expect(nextToggle).toHaveAttribute('aria-expanded', 'false');

  fireEvent.click(
    screen.getByRole('radio', {
      name: 'Успешные и неуспешные операции',
    }),
  );
  fireEvent.click(screen.getByTestId('answer-question-question-9'));

  await waitFor(() => {
    expect(currentToggle).toHaveAttribute('aria-expanded', 'false');
    expect(nextToggle).toHaveAttribute('aria-expanded', 'true');
    expect(nextToggle).toHaveFocus();
  });
  expect(scrollIntoView).toHaveBeenCalledWith({
    behavior: 'smooth',
    block: 'nearest',
  });
});

test('records proposal decisions and author comments as commands', async () => {
  renderPage();

  fireEvent.click(await screen.findByTestId('accept-proposal-proposal-12'));
  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenLastCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'DECIDE_PROPOSAL',
      payload: { proposal_id: 'proposal-12', decision: 'ACCEPTED' },
    }),
  );

  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Комментарий' })).toBeEnabled(),
  );
  expect(
    screen.getByTestId('business-document-comment-scope-selector'),
  ).toHaveTextContent('весь документ');
  expect(screen.getByRole('textbox', { name: 'Комментарий' })).toHaveAttribute(
    'placeholder',
    'Комментарий ко всему документу',
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Комментарий' }), {
    target: { value: 'Добавить метрику времени ответа' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(2));
  expect(mockedSubmit).toHaveBeenLastCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'ADD_COMMENT',
      payload: {
        text: 'Добавить метрику времени ответа',
        revision_id: 'revision-3',
        section_id: null,
        anchor: null,
      },
    }),
  );
});

test('binds a comment to text selected in the current revision', async () => {
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const [firstSection] = await screen.findAllByTestId(
    'business-document-section',
  );
  const selectionSpy = mockTextSelection(
    'Сократить время перевода',
    firstSection,
  );
  const startOffset = firstSectionText.indexOf('Сократить время перевода');
  const endOffset = startOffset + 'Сократить время перевода'.length;

  fireEvent.mouseUp(markdown);
  expect(
    screen.getByTestId('business-document-comment-scope-selector'),
  ).toHaveTextContent('выделенный фрагмент · § 1');
  expect(
    screen.getByTestId('business-document-comment-composer'),
  ).toHaveTextContent('Сократить время перевода');
  fireEvent.change(screen.getByRole('textbox', { name: 'Комментарий' }), {
    target: { value: 'Уточнить измеримый результат' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'ADD_COMMENT',
      payload: {
        text: 'Уточнить измеримый результат',
        revision_id: 'revision-3',
        section_id: '1',
        anchor: {
          revision_id: 'revision-3',
          section_id: '1',
          selected_text: 'Сократить время перевода',
          prefix: firstSectionText.slice(0, startOffset),
          suffix: firstSectionText.slice(endOffset, endOffset + 64),
          start_offset: startOffset,
          end_offset: endOffset,
        },
      },
    }),
  );
  selectionSpy.mockRestore();
});

test('labels a general protocol comment as applying to the whole document', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    protocol: {
      ...projection.protocol,
      comments: [
        {
          comment_id: 'comment-general',
          section_id: null,
          text: 'Унифицировать терминологию во всём документе',
          anchor_status: 'GENERAL',
          anchor: null,
        },
      ],
    },
  });
  renderPage();

  expect(
    (await screen.findAllByTestId('business-document-comment-scope'))[0],
  ).toHaveTextContent('Весь документ');
});

test('preserves the comment draft and anchor when submission fails', async () => {
  mockedSubmit.mockRejectedValueOnce(new Error('network down'));
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const [firstSection] = await screen.findAllByTestId(
    'business-document-section',
  );
  const selectionSpy = mockTextSelection(
    'Сократить время перевода',
    firstSection,
  );
  fireEvent.mouseUp(markdown);

  const comment = screen.getByRole('textbox', { name: 'Комментарий' });
  fireEvent.change(comment, {
    target: { value: 'Не потерять этот комментарий' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  expect(
    await screen.findByTestId('business-document-command-error'),
  ).toHaveTextContent('network down');
  expect(comment).toHaveValue('Не потерять этот комментарий');
  expect(
    screen.getByTestId('business-document-comment-composer'),
  ).toHaveTextContent('Сократить время перевода');
  selectionSpy.mockRestore();
});

test('anchors duplicate text to the selected canonical section', async () => {
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const sections = await screen.findAllByTestId('business-document-section');
  const selectionSpy = mockTextSelection('Повторяемая фраза', sections[1]);
  fireEvent.mouseUp(markdown);

  fireEvent.change(screen.getByRole('textbox', { name: 'Комментарий' }), {
    target: { value: 'Комментарий ко второму разделу' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  const startOffset = secondSectionText.indexOf('Повторяемая фраза');
  const endOffset = startOffset + 'Повторяемая фраза'.length;
  expect(mockedSubmit).toHaveBeenCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'ADD_COMMENT',
      payload: {
        text: 'Комментарий ко второму разделу',
        revision_id: 'revision-3',
        section_id: '2',
        anchor: {
          revision_id: 'revision-3',
          section_id: '2',
          selected_text: 'Повторяемая фраза',
          prefix: '',
          suffix: secondSectionText.slice(endOffset, endOffset + 64),
          start_offset: startOffset,
          end_offset: endOffset,
        },
      },
    }),
  );
  selectionSpy.mockRestore();
});

test('rejects a cross-section selection and submits a general comment', async () => {
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const sections = await screen.findAllByTestId('business-document-section');
  const selectionSpy = mockTextSelection(
    'Результат измеряется в минутах. Повторяемая фраза',
    sections[0],
    sections[1],
  );
  fireEvent.mouseUp(markdown);

  expect(
    screen.getByTestId('business-document-selection-error'),
  ).toHaveTextContent('Выделите фрагмент внутри одного раздела');
  expect(
    screen.getByTestId('business-document-comment-composer'),
  ).not.toHaveTextContent('Результат измеряется');

  fireEvent.change(screen.getByRole('textbox', { name: 'Комментарий' }), {
    target: { value: 'Общий комментарий после ошибки выделения' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'ADD_COMMENT',
      payload: {
        text: 'Общий комментарий после ошибки выделения',
        revision_id: 'revision-3',
        section_id: null,
        anchor: null,
      },
    }),
  );
  selectionSpy.mockRestore();
});

test('rejects text that is ambiguous inside its canonical section', async () => {
  const repeatedText = 'Одинаковый фрагмент. Одинаковый фрагмент.';
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    current_revision: {
      ...projection.current_revision!,
      document_ast: {
        ...projection.current_revision!.document_ast,
        sections: [
          {
            id: 'ambiguous',
            title: 'Повторы',
            blocks: [{ type: 'paragraph', text: repeatedText }],
          },
        ],
      },
      section_texts: { ambiguous: repeatedText },
      body_markdown: `## ambiguous. Повторы\n${repeatedText}`,
    },
  });
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const [section] = await screen.findAllByTestId('business-document-section');
  const selectionSpy = mockTextSelection('Одинаковый фрагмент', section);
  fireEvent.mouseUp(markdown);

  expect(
    screen.getByTestId('business-document-selection-error'),
  ).toHaveTextContent('Фрагмент повторяется в разделе');
  expect(
    screen.getByTestId('business-document-comment-composer'),
  ).not.toHaveTextContent('Одинаковый фрагмент');
  selectionSpy.mockRestore();
});

test('keeps emoji context windows within valid UTF-16 boundaries', async () => {
  const canonicalText = `${'A'.repeat(10)}😀${'B'.repeat(63)}TARGET${'C'.repeat(63)}😀tail`;
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    current_revision: {
      ...projection.current_revision!,
      document_ast: {
        ...projection.current_revision!.document_ast,
        sections: [
          {
            id: 'emoji',
            title: 'Составные символы',
            blocks: [{ type: 'paragraph', text: canonicalText }],
          },
        ],
      },
      section_texts: { emoji: canonicalText },
      body_markdown: `## emoji. Составные символы\n${canonicalText}`,
    },
  });
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const [section] = await screen.findAllByTestId('business-document-section');
  const selectionSpy = mockTextSelection('TARGET', section);
  fireEvent.mouseUp(markdown);

  fireEvent.change(screen.getByRole('textbox', { name: 'Комментарий' }), {
    target: { value: 'Проверить границы контекста' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  const payload = mockedSubmit.mock.calls[0][1].payload as {
    anchor: { prefix: string; suffix: string };
  };
  expect(payload.anchor.prefix).toBe('B'.repeat(63));
  expect(payload.anchor.suffix).toBe('C'.repeat(63));
  expect(payload.anchor.prefix).not.toContain('\ud83d');
  expect(payload.anchor.suffix).not.toContain('\ud83d');
  selectionSpy.mockRestore();
});

test('uses server-projected section text for exponent-float anchor parity', async () => {
  const canonicalText = '| Порог |\n| --- |\n| 1e-07 |';
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    current_revision: {
      ...projection.current_revision!,
      document_ast: {
        ...projection.current_revision!.document_ast,
        sections: [
          {
            id: 'numeric',
            title: 'Порог',
            blocks: [
              {
                type: 'table',
                headers: ['Порог'],
                rows: [[1e-7]],
              },
            ],
          },
        ],
      },
      section_texts: { numeric: canonicalText },
      body_markdown: `## numeric. Порог\n${canonicalText}`,
    },
  });
  renderPage();
  const markdown = await screen.findByTestId('business-document-markdown');
  const [section] = await screen.findAllByTestId('business-document-section');
  expect(section).toHaveAttribute('data-section-text', canonicalText);
  expect(section).toHaveTextContent('1e-07');

  const selectionSpy = mockTextSelection('1e-07', section);
  fireEvent.mouseUp(markdown);
  fireEvent.change(screen.getByRole('textbox', { name: 'Комментарий' }), {
    target: { value: 'Сохранить канонический формат числа' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Добавить комментарий' }));

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  const startOffset = canonicalText.indexOf('1e-07');
  expect(mockedSubmit.mock.calls[0][1]).toEqual(
    expect.objectContaining({
      type: 'ADD_COMMENT',
      payload: {
        text: 'Сохранить канонический формат числа',
        revision_id: 'revision-3',
        section_id: 'numeric',
        anchor: {
          revision_id: 'revision-3',
          section_id: 'numeric',
          selected_text: '1e-07',
          prefix: canonicalText.slice(0, startOffset),
          suffix: canonicalText.slice(startOffset + '1e-07'.length),
          start_offset: startOffset,
          end_offset: startOffset + '1e-07'.length,
        },
      },
    }),
  );
  selectionSpy.mockRestore();
});

test('shows a recoverable conflict instead of applying stale intent', async () => {
  mockedSubmit.mockRejectedValueOnce(
    new BusinessDocumentConflictError('stale state', 'STATE_VERSION_CONFLICT'),
  );
  renderPage();

  fireEvent.click(await screen.findByTestId('reject-proposal-proposal-12'));

  expect(
    await screen.findByTestId('business-document-conflict'),
  ).toHaveTextContent('Документ изменился в другой вкладке');
  expect(mockedFetch.mock.calls.length).toBeGreaterThanOrEqual(2);
});

test('shows domain conflicts without claiming the document changed elsewhere', async () => {
  mockedSubmit.mockRejectedValueOnce(
    new BusinessDocumentConflictError(
      'Есть открытые вопросы',
      'OPEN_REVIEW_QUESTIONS',
    ),
  );
  renderPage();

  fireEvent.click(await screen.findByTestId('reject-proposal-proposal-12'));

  expect(
    await screen.findByTestId('business-document-command-error'),
  ).toHaveTextContent('Есть открытые вопросы');
  expect(screen.queryByTestId('business-document-conflict')).toBeNull();
});

test('renders document and protocol empty states during intake', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'INTAKE',
    current_revision: null,
    active_review_cycle: 0,
    protocol: { questions: [], proposals: [], comments: [] },
    allowed_commands: [],
  });
  renderPage();

  expect(
    await screen.findByTestId('business-document-empty'),
  ).toHaveTextContent('Черновик ещё не создан');
  expect(
    screen.getByTestId('business-document-protocol-empty'),
  ).toHaveTextContent('Обсуждение пока пусто');
});

test('uses the canonical backend command to request a draft', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'INTAKE',
    current_revision: null,
    active_review_cycle: 0,
    protocol: { questions: [], proposals: [], comments: [] },
    allowed_commands: ['REQUEST_DRAFT'],
  });
  renderPage();

  fireEvent.click(
    await screen.findByRole('button', { name: 'Создать черновик' }),
  );

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenCalledWith(
    'doc-1',
    expect.objectContaining({ type: 'REQUEST_DRAFT', payload: {} }),
  );
});

test('uses an analysis action label for intake assessment', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    lifecycle_state: 'INTAKE',
    current_revision: null,
    active_review_cycle: 0,
    protocol: { questions: [], proposals: [], comments: [] },
    allowed_commands: ['REQUEST_INTAKE_ASSESSMENT'],
  });
  renderPage();

  expect(
    await screen.findByRole('button', {
      name: 'Проанализировать вводные',
    }),
  ).toBeInTheDocument();
});

test('requests review assessment before applying and exports EvaWiki explicitly', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    allowed_commands: ['REQUEST_REVIEW_ASSESSMENT'],
  });
  const { unmount } = renderPage();

  fireEvent.click(
    await screen.findByRole('button', { name: 'Проанализировать замечания' }),
  );
  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenLastCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'REQUEST_REVIEW_ASSESSMENT',
      payload: {},
    }),
  );

  unmount();
  jest.clearAllMocks();
  mockedFetch.mockResolvedValue({
    ...projection,
    lifecycle_state: 'AGREED',
    allowed_commands: ['REQUEST_EXPORT'],
  });
  mockedSubmit.mockResolvedValue(commandResult);
  renderPage();
  fireEvent.click(
    await screen.findByRole('button', { name: 'Создать HTML для EvaWiki' }),
  );

  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenLastCalledWith(
    'doc-1',
    expect.objectContaining({
      type: 'REQUEST_EXPORT',
      payload: { revision_id: 'revision-3', format: 'EVA_WIKI' },
    }),
  );
});

test('renders loading and retryable error states', async () => {
  let rejectRequest: (error: Error) => void = () => undefined;
  mockedFetch.mockImplementationOnce(
    () =>
      new Promise((_, reject) => {
        rejectRequest = reject;
      }),
  );
  renderPage();
  expect(screen.getByTestId('business-document-loading')).toBeInTheDocument();

  rejectRequest(new Error('network down'));
  expect(
    await screen.findByTestId('business-document-error'),
  ).toHaveTextContent('network down');
  fireEvent.click(screen.getByRole('button', { name: 'Повторить' }));
  await waitFor(() => expect(mockedFetch).toHaveBeenCalledTimes(2));
});

test('shows the active attempt and previous validation error while retrying', async () => {
  mockedFetch.mockResolvedValue({
    ...projection,
    operation_state: 'GENERATING_DRAFT',
    latest_job: {
      job_id: 'job-draft',
      job_type: 'GENERATE_DRAFT',
      status: 'RUNNING',
      progress: 0.62,
      progress_stage: 'GENERATING',
      progress_message: 'Формируем структуру и разделы',
      attempt: 2,
      max_attempts: 3,
      error: {
        code: 'INVALID_DRAFT_BUNDLE',
        message: 'Черновик не соответствует контракту',
      },
    },
  });

  renderPage();

  expect(
    await screen.findByTestId('business-document-operation'),
  ).toHaveTextContent('Попытка 2 из 3');
  expect(screen.getByTestId('business-document-operation')).toHaveTextContent(
    '62%',
  );
  expect(screen.getByTestId('business-document-operation')).toHaveTextContent(
    'Предыдущая ошибка: Черновик не соответствует контракту',
  );
});

test('creates a new business requirements document', async () => {
  renderPage('/business-documents');

  expect(screen.queryByText('Источники RAGFlow')).not.toBeInTheDocument();
  expect(
    screen.queryByTestId('business-document-datasets'),
  ).not.toBeInTheDocument();

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Название документа' }),
    {
      target: { value: '  Новый продукт  ' },
    },
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Описание идеи' }), {
    target: { value: 'Нужен новый клиентский сценарий.' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Начать работу' }));

  await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(1));
  expect(mockedCreate.mock.calls[0][0]).toEqual({
    schema_version: '2',
    document_type: 'business_requirements',
    title: '  Новый продукт  ',
    idea: 'Нужен новый клиентский сценарий.',
    dataset_ids: [],
  });
  await waitFor(() => expect(mockedSubmit).toHaveBeenCalledTimes(1));
  expect(mockedSubmit).toHaveBeenCalledWith(
    'doc-1',
    expect.objectContaining({
      schema_version: '1',
      expected_state_version: projection.state_version,
      type: 'REQUEST_INTAKE_ASSESSMENT',
      payload: {},
    }),
  );
  expect(mockedSubmit.mock.calls[0][1].command_id).toMatch(
    /^cmd-initial-analysis-/,
  );
  expect(mockedSubmit.mock.calls[0][1].idempotency_key).toMatch(
    /^idem-initial-analysis-/,
  );
  expect(
    await screen.findByTestId('business-document-workbench'),
  ).toBeVisible();
});

test('opens the created document when automatic analysis cannot be started', async () => {
  mockedSubmit.mockRejectedValueOnce(new Error('analysis unavailable'));
  renderPage('/business-documents');

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Название документа' }),
    { target: { value: 'Новый продукт' } },
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Описание идеи' }), {
    target: { value: 'Нужен новый клиентский сценарий.' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Начать работу' }));

  expect(
    await screen.findByTestId('business-document-workbench'),
  ).toBeVisible();
  expect(mockedCreate).toHaveBeenCalledTimes(1);
  expect(mockedSubmit).toHaveBeenCalledTimes(1);
});

test('does not show linked RAGFlow source counts in a document', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    dataset_ids: ['dataset-1'],
  });
  renderPage();

  expect(
    await screen.findByTestId('business-document-workbench'),
  ).toBeVisible();
  expect(screen.queryByText('Источников: 1')).not.toBeInTheDocument();
});

test('optionally links a new document to an EVA page URL', async () => {
  renderPage('/business-documents');

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Название документа' }),
    { target: { value: 'Связанный документ' } },
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Описание идеи' }), {
    target: { value: 'Синхронизировать требования с EVA.' },
  });
  fireEvent.change(screen.getByRole('textbox', { name: 'URL страницы EVA' }), {
    target: {
      value: 'https://eva.example.com/project/Document/BR-42',
    },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Начать работу' }));

  expect(
    await screen.findByTestId('eva-replace-confirmation'),
  ).toHaveTextContent('текущее содержимое страницы');
  fireEvent.click(screen.getByTestId('confirm-eva-replace'));

  await waitFor(() =>
    expect(mockedCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        eva_page_url: 'https://eva.example.com/project/Document/BR-42',
        eva_decision: { mode: 'BIND', confirm_replace: true },
      }),
      expect.anything(),
    ),
  );
});

test('offers matching EVA pages and confirms replacement before binding', async () => {
  mockedCreate
    .mockRejectedValueOnce(
      new BusinessDocumentConflictError(
        'Выберите страницу',
        'EVA_BINDING_DECISION_REQUIRED',
        {
          matches: [
            {
              connector_id: 'connector-1',
              connector_name: 'EVA Wiki',
              eva_origin: 'https://eva-api.example.com',
              id: 'CmfDocument:doc-42',
              name: 'Новый продукт',
              code: 'BR-42',
              project_id: 'CmfProject:portal',
              web_url: 'https://eva.example.com/project/Document/BR-42',
              breadcrumbs: [
                {
                  id: 'CmfDocument:folder',
                  name: 'Проекты',
                  web_url: 'https://eva.example.com/project/Document/PROJECTS',
                },
                {
                  id: 'CmfDocument:doc-42',
                  name: 'Новый продукт',
                  web_url: 'https://eva.example.com/project/Document/BR-42',
                },
              ],
              hierarchy: 'Проекты › Новый продукт',
              binding_available: true,
            },
            {
              connector_id: 'connector-1',
              connector_name: 'EVA Wiki',
              eva_origin: 'https://eva-api.example.com',
              id: 'CmfDocument:doc-occupied',
              name: 'Новый продукт',
              code: 'BR-99',
              project_id: 'CmfProject:archive',
              web_url: 'https://eva.example.com/archive/Document/BR-99',
              breadcrumbs: [
                {
                  id: 'CmfDocument:archive',
                  name: 'Архив',
                  web_url: 'https://eva.example.com/archive/Document/ARCHIVE',
                },
                {
                  id: 'CmfDocument:doc-occupied',
                  name: 'Новый продукт',
                  web_url: 'https://eva.example.com/archive/Document/BR-99',
                },
              ],
              hierarchy: 'Архив › Новый продукт',
              binding_available: false,
              linked_document: {
                document_id: 'existing-document',
                title: 'Уже созданный документ',
              },
            },
          ],
        },
      ),
    )
    .mockResolvedValueOnce(projection);
  renderPage('/business-documents');

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Название документа' }),
    { target: { value: 'Новый продукт' } },
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Описание идеи' }), {
    target: { value: 'Нужен новый клиентский сценарий.' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Начать работу' }));

  expect(await screen.findByTestId('eva-title-match-banner')).toHaveTextContent(
    'Проекты',
  );
  expect(screen.getByTestId('eva-title-match-banner')).toHaveTextContent(
    'Уже привязана к документу «Уже созданный документ»',
  );
  expect(screen.getByRole('link', { name: 'Проекты' })).toHaveAttribute(
    'href',
    'https://eva.example.com/project/Document/PROJECTS',
  );
  expect(screen.getByRole('link', { name: 'Архив' })).toHaveAttribute(
    'href',
    'https://eva.example.com/archive/Document/ARCHIVE',
  );
  const bindButtons = screen.getAllByRole('button', { name: 'Привязать' });
  expect(bindButtons).toHaveLength(2);
  expect(bindButtons[1]).toBeDisabled();
  fireEvent.click(bindButtons[0]);
  expect(
    await screen.findByTestId('eva-replace-confirmation'),
  ).toHaveTextContent('полностью заменено содержимым документа');
  fireEvent.click(screen.getByRole('button', { name: 'Отмена' }));
  await waitFor(() =>
    expect(
      screen.queryByTestId('eva-replace-confirmation'),
    ).not.toBeInTheDocument(),
  );
  expect(mockedCreate).toHaveBeenCalledTimes(1);

  fireEvent.click(bindButtons[0]);
  fireEvent.click(screen.getByTestId('confirm-eva-replace'));

  await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(2));
  expect(mockedCreate.mock.calls[1][0]).toEqual(
    expect.objectContaining({
      schema_version: '2',
      eva_page_url: 'https://eva.example.com/project/Document/BR-42',
      eva_decision: { mode: 'BIND', confirm_replace: true },
    }),
  );
});

test('creates without EVA binding after the user explicitly skips matches', async () => {
  mockedCreate
    .mockRejectedValueOnce(
      new BusinessDocumentConflictError(
        'Выберите страницу',
        'EVA_BINDING_DECISION_REQUIRED',
        {
          matches: [
            {
              connector_id: 'connector-1',
              connector_name: 'EVA Wiki',
              eva_origin: 'https://eva-api.example.com',
              id: 'CmfDocument:doc-42',
              name: 'Новый продукт',
              code: 'BR-42',
              project_id: 'CmfProject:portal',
              web_url: 'https://eva.example.com/project/Document/BR-42',
              breadcrumbs: [],
              hierarchy: 'Проекты › Новый продукт',
              binding_available: true,
            },
          ],
        },
      ),
    )
    .mockResolvedValueOnce(projection);
  renderPage('/business-documents');

  fireEvent.change(
    screen.getByRole('textbox', { name: 'Название документа' }),
    { target: { value: 'Новый продукт' } },
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Описание идеи' }), {
    target: { value: 'Нужен новый клиентский сценарий.' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Начать работу' }));
  fireEvent.click(await screen.findByTestId('create-without-eva-binding'));

  await waitFor(() => expect(mockedCreate).toHaveBeenCalledTimes(2));
  expect(mockedCreate.mock.calls[1][0]).toEqual(
    expect.objectContaining({
      schema_version: '2',
      eva_decision: { mode: 'SKIP' },
    }),
  );
  expect(mockedCreate.mock.calls[1][0]).not.toHaveProperty('eva_page_url');
});

test('appends voice transcripts to document input fields', () => {
  renderPage('/business-documents');

  const title = screen.getByRole('textbox', { name: 'Название документа' });
  const idea = screen.getByRole('textbox', { name: 'Описание идеи' });
  fireEvent.change(title, { target: { value: 'Новый' } });
  fireEvent.change(idea, { target: { value: 'Исходная идея.' } });

  fireEvent.click(screen.getByTestId('voice-input-document-title'));
  fireEvent.click(screen.getByTestId('voice-input-document-idea'));

  expect(title).toHaveValue('Новый текст из диктовки');
  expect(idea).toHaveValue('Исходная идея. текст из диктовки');
});

test('lists saved documents so work can be resumed', async () => {
  mockedList.mockResolvedValueOnce({
    items: [
      {
        document_id: 'saved-1',
        title: 'Сохранённые требования',
        lifecycle_state: 'AGREED',
        operation_state: 'IDLE',
        state_version: 9,
        current_revision_number: 2,
        update_time: 1_787_695_200,
      },
    ],
    total: 1,
    page: 1,
    page_size: 20,
  });
  renderPage('/business-documents');

  expect(
    await screen.findByTestId('business-document-list-item'),
  ).toHaveTextContent('Сохранённые требования');
  expect(screen.getByTestId('business-document-list-item')).toHaveTextContent(
    'Ревизия 2',
  );
  fireEvent.click(screen.getByTestId('business-document-list-item'));
  await waitFor(() => expect(mockedFetch).toHaveBeenCalledWith('saved-1'));
});

test('shows compact processing progress in the document list', async () => {
  mockedList.mockResolvedValueOnce({
    items: [
      {
        document_id: 'running-1',
        title: 'Регламент закупочной деятельности',
        lifecycle_state: 'INTAKE',
        operation_state: 'GENERATING_DRAFT',
        state_version: 4,
        current_revision_number: null,
        update_time: 1_788_451_200,
        latest_job: {
          job_id: 'job-running',
          job_type: 'GENERATE_DRAFT',
          status: 'RUNNING',
          progress: 0.62,
          progress_stage: 'GENERATING',
          progress_message: 'Формируем структуру и разделы',
          attempt: 1,
          max_attempts: 3,
        },
      },
    ],
    total: 1,
    page: 1,
    page_size: 20,
  });
  renderPage('/business-documents');

  const progress = await screen.findByTestId('business-document-list-progress');
  expect(progress).toHaveTextContent('Формируем структуру и разделы');
  expect(progress).toHaveTextContent('62%');
  expect(
    screen.getByRole('progressbar', { name: 'Прогресс обработки: 62%' }),
  ).toBeInTheDocument();
});

test('switches between my and all documents', async () => {
  renderPage('/business-documents');

  await waitFor(() => expect(mockedList).toHaveBeenCalledWith(1, 20, 'mine'));
  fireEvent.click(screen.getByTestId('business-documents-filter-all'));
  await waitFor(() => expect(mockedList).toHaveBeenCalledWith(1, 20, 'all'));
});

test('does not offer document creation to an author-editor', async () => {
  mockedList.mockResolvedValueOnce({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
    access_role: 'AUTHOR_EDITOR',
    capabilities: {
      read: true,
      create: false,
      edit_own: true,
      edit_all: false,
      delete: false,
      assign: false,
    },
  });
  renderPage('/business-documents');

  expect(
    await screen.findByTestId('business-document-create-denied'),
  ).toHaveTextContent('Создание документов недоступно');
  expect(screen.getByTestId('new-document-mode')).toBeDisabled();
  expect(
    screen.queryByRole('button', { name: 'Начать работу' }),
  ).not.toBeInTheDocument();
});

test('allows an extended moderator to assign a document owner', async () => {
  const assigned = {
    ...projection,
    owner_id: 'author-2',
    owner_name: 'Второй автор',
    access_role: 'EXTENDED_MODERATOR' as const,
    permissions: { read: true, edit: true, delete: true, assign: true },
  };
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    owner_id: 'author-1',
    owner_name: 'Первый автор',
    access_role: 'EXTENDED_MODERATOR',
    permissions: { read: true, edit: true, delete: true, assign: true },
  });
  mockedListAccessUsers.mockResolvedValueOnce({
    items: [
      {
        user_id: 'author-1',
        nickname: 'Первый автор',
        email: 'author-1@example.com',
        role: 'AUTHOR_CREATOR',
      },
      {
        user_id: 'author-2',
        nickname: 'Второй автор',
        email: 'author-2@example.com',
        role: 'AUTHOR_EDITOR',
      },
    ],
  });
  mockedAssignOwner.mockResolvedValueOnce(assigned);
  renderPage('/business-documents/doc-1');

  fireEvent.click(await screen.findByTestId('business-document-owner-select'));
  expect(screen.getByText('Владелец: Первый автор')).toBeVisible();
  expect(
    screen.getByTestId('business-document-owner-option-author-2'),
  ).toHaveTextContent('Второй автор (author-2@example.com)');
  fireEvent.click(
    await screen.findByTestId('business-document-owner-option-author-2'),
  );
  fireEvent.click(screen.getByTestId('business-document-assign-owner'));

  await waitFor(() =>
    expect(mockedAssignOwner).toHaveBeenCalledWith('doc-1', 'author-2', 18),
  );
  expect(await screen.findByText('Владелец: Второй автор')).toBeVisible();
});

test('shows hard delete only for an administrator and confirms it', async () => {
  mockedList.mockResolvedValueOnce({
    items: [
      {
        document_id: 'saved-1',
        owner_id: 'author-1',
        owner_name: 'Первый автор',
        access_role: 'ADMIN',
        permissions: { read: true, edit: true, delete: true },
        title: 'Документ другого автора',
        lifecycle_state: 'AGREED',
        operation_state: 'IDLE',
        state_version: 9,
        current_revision_number: 2,
        update_time: 1_787_695_200,
      },
    ],
    total: 1,
    page: 1,
    page_size: 20,
  });
  renderPage('/business-documents');

  expect(await screen.findByText('Владелец: Первый автор')).toBeVisible();
  expect(screen.queryByText('Владелец: author-1')).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole('button', {
      name: 'Удалить документ «Документ другого автора»',
    }),
  );
  fireEvent.click(await screen.findByRole('button', { name: /^Удалить$/ }));

  await waitFor(() => expect(mockedDelete).toHaveBeenCalledWith('saved-1'));
});

test('keeps a foreign document editable for admin and allows deletion from its workbench', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    owner_id: 'author-1',
    access_role: 'ADMIN',
    permissions: { read: true, edit: true, delete: true },
  });
  renderPage('/business-documents/doc-1');

  expect(
    await screen.findByTestId('business-document-delete-detail'),
  ).toBeVisible();
  expect(screen.getByText('Применить исправления')).toBeVisible();
  fireEvent.click(screen.getByTestId('business-document-delete-detail'));
  const dialog = await screen.findByRole('alertdialog');
  fireEvent.click(within(dialog).getByRole('button', { name: /^Удалить$/ }));

  await waitFor(() => expect(mockedDelete).toHaveBeenCalledWith('doc-1'));
});

test('keeps a foreign document read-only for an author', async () => {
  mockedFetch.mockResolvedValueOnce({
    ...projection,
    owner_id: 'author-1',
    access_role: 'AUTHOR_CREATOR',
    permissions: { read: true, edit: false, delete: false },
  });
  renderPage('/business-documents/doc-1');

  expect(await screen.findByText(projection.title)).toBeVisible();
  expect(screen.queryByText('Применить исправления')).not.toBeInTheDocument();
  expect(
    screen.queryByTestId('business-document-delete-detail'),
  ).not.toBeInTheDocument();
});

test('shows the reviewed lifecycle and review operation labels', async () => {
  mockedList.mockResolvedValueOnce({
    items: [
      {
        document_id: 'review-1',
        title: 'Документ на внесении изменений',
        lifecycle_state: 'REVIEW',
        operation_state: 'ANALYZING_REVIEW',
        state_version: 3,
        current_revision_number: 1,
        update_time: 1_787_695_200,
      },
      {
        document_id: 'agreed-1',
        title: 'Проверенный документ',
        lifecycle_state: 'AGREED',
        operation_state: 'IDLE',
        state_version: 4,
        current_revision_number: 2,
        update_time: 1_787_695_200,
      },
      {
        document_id: 'archived-1',
        title: 'Архивный документ',
        lifecycle_state: 'ARCHIVED',
        operation_state: 'IDLE',
        state_version: 5,
        current_revision_number: 2,
        update_time: 1_787_695_200,
      },
    ],
    total: 3,
    page: 1,
    page_size: 20,
  });
  renderPage('/business-documents');

  expect(await screen.findByText('Внесение изменений')).toBeInTheDocument();
  expect(screen.getByText('Правки внесены')).toBeInTheDocument();
  expect(screen.getByText('В архиве')).toBeInTheDocument();
  expect(screen.getByText('Анализ замечаний')).toBeInTheDocument();
});

test('finds an existing EVA document and opens a pinned change request', async () => {
  mockedSearchEva.mockResolvedValueOnce({
    connectors: [{ connector_id: 'connector-1', connector_name: 'EVA Wiki' }],
    items: [
      {
        connector_id: 'connector-1',
        connector_name: 'EVA Wiki',
        id: 'CmfDocument:doc-1',
        name: 'Переводы одной кнопкой',
        code: 'BR-42',
        project_id: 'project-1',
        version: '1|version-1|2026-08-26',
        modified_at: '2026-08-26T09:00:00+03:00',
        web_url: 'https://eva.example.com/project/Document/BR-42',
        excerpt: 'Существующие бизнес-требования',
      },
    ],
  });
  mockedCreateEva.mockResolvedValueOnce(evaChange);
  renderPage('/business-documents');

  fireEvent.click(screen.getByTestId('eva-change-mode'));
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Поиск документа EVA' }),
    { target: { value: 'BR-42' } },
  );
  fireEvent.click(screen.getByRole('button', { name: 'Найти документ EVA' }));
  fireEvent.click(await screen.findByTestId('eva-source-result'));
  fireEvent.click(screen.getByRole('button', { name: 'Проверить' }));
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Описание доработки' }),
    { target: { value: 'Уточнить ожидаемый результат.' } },
  );
  fireEvent.click(
    screen.getByRole('button', { name: 'Подготовить доработку' }),
  );

  await waitFor(() => expect(mockedCreateEva).toHaveBeenCalledTimes(1));
  expect(mockedCreateEva.mock.calls[0][0]).toEqual({
    connector_id: 'connector-1',
    document_id: 'CmfDocument:doc-1',
    change_summary:
      'Сценарий: проверить документ на противоречия, пропуски и неясные требования, затем подготовить исправленный вариант в границах задачи автора.\n\nЗадача автора:\nУточнить ожидаемый результат.',
  });
  expect(await screen.findByTestId('eva-change-workbench')).toBeVisible();
  expect(mockedFetchEva).toHaveBeenCalledWith('change-1');
});

test('shows a read-only agent draft and regenerates it only from instructions', async () => {
  renderPage('/business-documents/eva/change-1');

  expect(await screen.findByTestId('eva-change-workbench')).toBeVisible();
  expect(screen.getByTestId('eva-change-diff')).toHaveTextContent(
    'Старый текст.',
  );
  expect(screen.getByTestId('eva-change-diff')).toHaveTextContent(
    'Новый текст.',
  );
  const confirmationPane = screen.getByRole('complementary', {
    name: 'Подтвердить запись в EVA',
  });
  expect(
    within(confirmationPane).getByRole('heading', {
      name: 'Подтвердить запись в EVA',
    }),
  ).toBeVisible();
  expect(
    within(
      within(confirmationPane).getByRole('list', {
        name: 'Этапы публикации в EVA',
      }),
    ).getAllByRole('listitem'),
  ).toHaveLength(3);
  expect(
    within(confirmationPane).getByText('Сформировать черновик'),
  ).toBeVisible();
  expect(
    within(confirmationPane).getByText('Сохранить как черновик в EVA'),
  ).toBeVisible();
  expect(
    within(confirmationPane).getByRole('button', {
      name: 'Подтвердить изменения',
    }),
  ).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Вариант агента' }));
  expect(screen.getByTestId('eva-agent-draft')).toHaveTextContent(
    'Новый текст.',
  );
  expect(
    screen.queryByRole('textbox', { name: 'Черновик документа EVA' }),
  ).not.toBeInTheDocument();
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Уточнение доработки EVA' }),
    { target: { value: 'Сделать цель измеримой.' } },
  );
  fireEvent.click(screen.getByTestId('generate-eva-change-draft'));

  await waitFor(() =>
    expect(mockedGenerateEva).toHaveBeenCalledWith('change-1', {
      expected_state_version: 2,
      refinement: 'Сделать цель измеримой.',
    }),
  );
  expect(mockedPrepareEva).not.toHaveBeenCalled();
  expect(mockedPublishEva).not.toHaveBeenCalled();
  expect(mockedApproveEva).not.toHaveBeenCalled();
});

test('asks for confirmation and force-overwrites a changed EVA document', async () => {
  const approvedChange: EvaDocumentChange = {
    ...evaChange,
    state_version: 4,
    workflow_state: 'APPROVED',
    allowed_actions: ['PREPARE_EVA_DRAFT'],
  };
  mockedFetchEva.mockResolvedValue(approvedChange);
  mockedPrepareEva
    .mockRejectedValueOnce(
      new BusinessDocumentConflictError(
        'The published EVA document changed after this change request was created',
        'EVA_SOURCE_VERSION_CONFLICT',
        {
          confirmation_required: true,
          confirmation_action: 'OVERWRITE_EVA_DOCUMENT',
        },
      ),
    )
    .mockResolvedValueOnce({
      ...approvedChange,
      state_version: 6,
      workflow_state: 'EVA_DRAFT_READY',
      allowed_actions: ['PUBLISH_EVA'],
    });

  renderPage('/business-documents/eva/change-1');
  const prepareDraftButton = await screen.findByTestId('prepare-eva-draft');
  expect(screen.getByText('Изменения подтверждены')).toBeVisible();
  expect(prepareDraftButton).toHaveTextContent('Сохранить как черновик в EVA');
  fireEvent.click(prepareDraftButton);

  expect(await screen.findByText('Перезаписать документ EVA?')).toBeVisible();
  expect(mockedPrepareEva).toHaveBeenNthCalledWith(1, 'change-1', 4, false);

  fireEvent.click(screen.getByTestId('confirm-eva-overwrite'));

  await waitFor(() =>
    expect(mockedPrepareEva).toHaveBeenNthCalledWith(2, 'change-1', 4, true),
  );
});
