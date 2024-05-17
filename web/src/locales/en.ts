export default {
  translation: {
    common: {
      delete: 'Delete',
      deleteModalTitle: 'Are you sure delete this item?',
      ok: 'Yes',
      cancel: 'No',
      total: 'Total',
      rename: 'Rename',
      name: 'Name',
      save: 'Save',
      namePlaceholder: 'Please input name',
      next: 'Next',
      create: 'Create',
      edit: 'Edit',
      upload: 'Upload',
      english: 'English',
      chinese: 'Simplified Chinese',
      traditionalChinese: 'Traditional Chinese',
      language: 'Language',
      languageMessage: 'Please input your language!',
      languagePlaceholder: 'select your language',
      copy: 'Copy',
      copied: 'Copied',
      comingSoon: 'Coming Soon',
      download: 'Download',
      close: 'Close',
      preview: 'Preview',
    },
    login: {
      login: 'Sign in',
      signUp: 'Sign up',
      loginDescription: 'We’re so excited to see you again!',
      registerDescription: 'Glad to have you on board!',
      emailLabel: 'Email',
      emailPlaceholder: 'Please input email',
      passwordLabel: 'Password',
      passwordPlaceholder: 'Please input password',
      rememberMe: 'Remember me',
      signInTip: 'Don’t have an account?',
      signUpTip: 'Already have an account?',
      nicknameLabel: 'Nickname',
      nicknamePlaceholder: 'Please input nickname',
      register: 'Create an account',
      continue: 'Continue',
      title: 'Start building your smart assistants.',
      description:
        'Sign up for free to explore top RAG technology. Create knowledge bases and AIs to empower your business.',
      review: 'from 500+ reviews',
    },
    header: {
      knowledgeBase: 'Knowledge Base',
      chat: 'Chat',
      register: 'Register',
      signin: 'Sign in',
      home: 'Home',
      setting: '用户设置',
      logout: '登出',
      fileManager: 'File Management',
    },
    knowledgeList: {
      welcome: 'Welcome back',
      description: 'Which knowledge base are we going to use today?',
      createKnowledgeBase: 'Create knowledge base',
      name: 'Name',
      namePlaceholder: 'Please input name!',
      doc: 'Docs',
      searchKnowledgePlaceholder: 'Search',
    },
    knowledgeDetails: {
      dataset: 'Dataset',
      testing: 'Retrieval testing',
      files: 'files',
      configuration: 'Configuration',
      name: 'Name',
      namePlaceholder: 'Please input name!',
      doc: 'Docs',
      datasetDescription:
        '😉 Questions and answers can only be answered after the parsing is successful.',
      addFile: 'Add file',
      searchFiles: 'Search your files',
      localFiles: 'Local files',
      emptyFiles: 'Create empty file',
      chunkNumber: 'Chunk Number',
      uploadDate: 'Upload Date',
      chunkMethod: 'Chunk Method',
      enabled: 'Enable',
      disabled: 'Disable',
      action: 'Action',
      parsingStatus: 'Parsing Status',
      processBeginAt: 'Process Begin At',
      processDuration: 'Process Duration',
      progressMsg: 'Progress Msg',
      testingDescription:
        'Final step! After success, leave the rest to Infiniflow AI.',
      topK: 'Top K',
      topKTip:
        "For the computaion cost, not all the retrieved chunk will be computed vector cosine similarity with query. The bigger the 'Top K' is, the higher the recall rate is, the slower the retrieval speed is.",
      similarityThreshold: 'Similarity threshold',
      similarityThresholdTip:
        "We use hybrid similarity score to evaluate distance between two lines of text. It's weighted keywords similarity and vector cosine similarity. If the similarity between query and chunk is less than this threshold, the chunk will be filtered out.",
      vectorSimilarityWeight: 'Vector similarity weight',
      vectorSimilarityWeightTip:
        "We use hybrid similarity score to evaluate distance between two lines of text. It's weighted keywords similarity and vector cosine similarity. The sum of both weights is 1.0.",
      testText: 'Test text',
      testTextPlaceholder: 'Please input your question!',
      testingLabel: 'Testing',
      similarity: 'Hybrid Similarity',
      termSimilarity: 'Term Similarity',
      vectorSimilarity: 'Vector Similarity',
      hits: 'Hits',
      view: 'View',
      filesSelected: 'Files Selected',
      upload: 'Upload',
      run: 'Run',
      runningStatus0: 'UNSTART',
      runningStatus1: 'Parsing',
      runningStatus2: 'CANCEL',
      runningStatus3: 'SUCCESS',
      runningStatus4: 'FAIL',
      pageRanges: 'Page Ranges',
      pageRangesTip:
        'page ranges: Define the page ranges that need to be parsed. The pages that not included in these ranges will be ignored.',
      fromPlaceholder: 'from',
      fromMessage: 'Missing start page number',
      toPlaceholder: 'to',
      toMessage: 'Missing end page number(excluded)',
      layoutRecognize: 'Layout recognize',
      layoutRecognizeTip:
        'Use visual models for layout analysis to better identify document structure, find where the titles, text blocks, images, and tables are. Without this feature, only the plain text of the PDF can be obtained.',
      taskPageSize: 'Task page size',
      taskPageSizeMessage: 'Please input your task page size!',
      taskPageSizeTip: `If using layout recognize, the PDF file will be split into groups of successive. Layout analysis will be performed parallelly between groups to increase the processing speed. The 'Task page size' determines the size of groups. The larger the page size is, the lower the chance of splitting continuous text between pages into different chunks.`,
      addPage: 'Add page',
      greaterThan: 'The current value must be greater than to!',
      greaterThanPrevious:
        'The current value must be greater than the previous to!',
      selectFiles: 'Select files',
      changeSpecificCategory: 'Change specific category',
      uploadTitle: 'Click or drag file to this area to upload',
      uploadDescription:
        'Support for a single or bulk upload. Strictly prohibited from uploading company data or other banned files.',
      chunk: 'Chunk',
      bulk: 'Bulk',
      cancel: 'Cancel',
    },
    knowledgeConfiguration: {
      titleDescription:
        'Update your knowledge base details especially parsing method here.',
      name: 'Knowledge base name',
      photo: 'Knowledge base photo',
      description: 'Description',
      language: 'Language',
      languageMessage: 'Please input your language!',
      languagePlaceholder: 'Please input your language!',
      permissions: 'Permissions',
      embeddingModel: 'Embedding model',
      chunkTokenNumber: 'Chunk token number',
      chunkTokenNumberMessage: 'Chunk token number is required',
      embeddingModelTip:
        "The embedding model used to embedding chunks. It's unchangable once the knowledgebase has chunks. You need to delete all the chunks if you want to change it.",
      permissionsTip:
        "If the permission is 'Team', all the team member can manipulate the knowledgebase.",
      chunkTokenNumberTip:
        'It determine the token number of a chunk approximately.',
      chunkMethod: 'Chunk method',
      chunkMethodTip: 'The instruction is at right.',
      upload: 'Upload',
      english: 'English',
      chinese: 'Chinese',
      embeddingModelPlaceholder: 'Please select a embedding model',
      chunkMethodPlaceholder: 'Please select a chunk method',
      save: 'Save',
      me: 'Only me',
      team: 'Team',
      cancel: 'Cancel',
      methodTitle: 'Chunking Method Description',
      methodExamples: 'Examples',
      methodExamplesDescription:
        'The following screenshots are presented to facilitate understanding.',
      dialogueExamplesTitle: 'Dialogue Examples',
      methodEmpty:
        'This will display a visual explanation of the knowledge base categories',
      book: `<p>Supported file formats are <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.</p><p>
      Since a book is long and not all the parts are useful, if it's a PDF,
      please setup the <i>page ranges</i> for every book in order eliminate negative effects and save computing time for analyzing.</p>`,
      laws: `<p>Supported file formats are <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.</p><p>
      Legal documents have a very rigorous writing format. We use text feature to detect split point. 
      </p><p>
      The chunk granularity is consistent with 'ARTICLE', and all the upper level text will be included in the chunk.
      </p>`,
      manual: `<p>Only <b>PDF</b> is supported.</p><p>
      We assume manual has hierarchical section structure. We use the lowest section titles as pivots to slice documents.
      So, the figures and tables in the same section will not be sliced apart, and chunk size might be large.
      </p>`,
      naive: `<p>Supported file formats are <b>DOCX, EXCEL, PPT, IMAGE, PDF, TXT</b>.</p>
      <p>This method apply the naive ways to chunk files: </p>
      <p>
      <li>Successive text will be sliced into pieces using vision detection model.</li>
      <li>Next, these successive pieces are merge into chunks whose token number is no more than 'Token number'.</li></p>`,
      paper: `<p>Only <b>PDF</b> file is supported.</p><p>
      If our model works well, the paper will be sliced by it's sections, like <i>abstract, 1.1, 1.2</i>, etc. </p><p>
      The benefit of doing this is that LLM can better summarize the content of relevant sections in the paper, 
      resulting in more comprehensive answers that help readers better understand the paper. 
      The downside is that it increases the context of the LLM conversation and adds computational cost, 
      so during the conversation, you can consider reducing the ‘<b>topN</b>’ setting.</p>`,
      presentation: `<p>The supported file formats are <b>PDF</b>, <b>PPTX</b>.</p><p>
      Every page will be treated as a chunk. And the thumbnail of every page will be stored.</p><p>
      <i>All the PPT files you uploaded will be chunked by using this method automatically, setting-up for every PPT file is not necessary.</i></p>`,
      qa: `
      <p>
      This chunk method supports <b>EXCEL</b> and <b>CSV/TXT</b> file formats.
    </p>
    <li>
      If the file is in <b>Excel</b> format, it should consist of two columns
      without headers: one for questions and the other for answers, with the
      question column preceding the answer column. Multiple sheets are
      acceptable as long as the columns are correctly structured.
    </li>
    <li>
      If the file is in <b>CSV/TXT</b> format, it must be UTF-8 encoded with TAB
      used as the delimiter to separate questions and answers.
    </li>
    <p>
      <i>
        Lines of texts that fail to follow the above rules will be ignored, and
        each Q&A pair will be considered a distinct chunk.
      </i>
    </p>
      `,
      resume: `<p>The supported file formats are <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.
      </p><p>
      The résumé comes in a variety of formats, just like a person’s personality, but we often have to organize them into structured data that makes it easy to search.
      </p><p>
      Instead of chunking the résumé, we parse the résumé into structured data. As a HR, you can dump all the résumé you have, 
      the you can list all the candidates that match the qualifications just by talk with <i>'RAGFlow'</i>.
      </p>
      `,
      table: `<p><b>EXCEL</b> and <b>CSV/TXT</b> format files are supported.</p><p>
      Here're some tips:
      <ul>
    <li>For csv or txt file, the delimiter between columns is <em><b>TAB</b></em>.</li>
    <li>The first line must be column headers.</li>
    <li>Column headers must be meaningful terms in order to make our LLM understanding.
    It's good to enumerate some synonyms using slash <i>'/'</i> to separate, and even better to
    enumerate values using brackets like <i>'gender/sex(male, female)'</i>.<p>
    Here are some examples for headers:<ol>
        <li>supplier/vendor<b>'TAB'</b>color(yellow, red, brown)<b>'TAB'</b>gender/sex(male, female)<b>'TAB'</b>size(M,L,XL,XXL)</li>
        <li>姓名/名字<b>'TAB'</b>电话/手机/微信<b>'TAB'</b>最高学历（高中，职高，硕士，本科，博士，初中，中技，中专，专科，专升本，MPA，MBA，EMBA）</li>
        </ol>
        </p>
    </li>
    <li>Every row in table will be treated as a chunk.</li>
    </ul>`,
      picture: `
    <p>Image files are supported. Video is coming soon.</p><p>
    If the picture has text in it, OCR is applied to extract the text as its text description.
    </p><p>
    If the text extracted by OCR is not enough, visual LLM is used to get the descriptions.
    </p>`,
      one: `
    <p>Supported file formats are <b>DOCX, EXCEL, PDF, TXT</b>.
    </p><p>
    For a document, it will be treated as an entire chunk, no split at all.
    </p><p>
    If you want to summarize something that needs all the context of an article and the selected LLM's context length covers the document length, you can try this method.
    </p>`,
    },
    chunk: {
      chunk: 'Chunk',
      bulk: 'Bulk',
      selectAll: 'Select All',
      enabledSelected: 'Enable Selected',
      disabledSelected: 'Disable Selected',
      deleteSelected: 'Delete Selected',
      search: 'Search',
      all: 'All',
      enabled: 'Enabled',
      disabled: 'Disabled',
      keyword: 'Keyword',
      function: 'Function',
      chunkMessage: 'Please input value!',
      full: 'Full text',
      ellipse: 'Ellipse',
    },
    chat: {
      createAssistant: 'Create an Assistant',
      assistantSetting: 'Assistant Setting',
      promptEngine: 'Prompt Engine',
      modelSetting: 'Model Setting',
      chat: 'Chat',
      newChat: 'New chat',
      send: 'Send',
      sendPlaceholder: 'Message Resume Assistant...',
      chatConfiguration: 'Chat Configuration',
      chatConfigurationDescription:
        ' Here, dress up a dedicated assistant for your special knowledge bases! 💕',
      assistantName: 'Assistant name',
      assistantNameMessage: 'Assistant name is required',
      namePlaceholder: 'e.g. Resume Jarvis',
      assistantAvatar: 'Assistant avatar',
      language: 'Language',
      emptyResponse: 'Empty response',
      emptyResponseTip: `If nothing is retrieved with user's question in the knowledgebase, it will use this as an answer. If you want LLM comes up with its own opinion when nothing is retrieved, leave this blank.`,
      setAnOpener: 'Set an opener',
      setAnOpenerInitial: `Hi! I'm your assistant, what can I do for you?`,
      setAnOpenerTip: 'How do you want to welcome your clients?',
      knowledgeBases: 'Knowledgebases',
      knowledgeBasesMessage: 'Please select',
      knowledgeBasesTip: 'Select knowledgebases associated.',
      system: 'System',
      systemInitialValue: `You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, your answer must include the sentence "The answer you are looking for is not found in the knowledge base!" Answers need to consider chat history.
      Here is the knowledge base:
      {knowledge}
      The above is the knowledge base.`,
      systemMessage: 'Please input!',
      systemTip:
        'Instructions you need LLM to follow when LLM answers questions, like charactor design, answer length and answer language etc.',
      topN: 'Top N',
      topNTip: `Not all the chunks whose similarity score is above the 'simialrity threashold' will be feed to LLMs. LLM can only see these 'Top N' chunks.`,
      variable: 'Variable',
      variableTip: `If you use dialog APIs, the varialbes might help you chat with your clients with different strategies. 
      The variables are used to fill-in the 'System' part in prompt in order to give LLM a hint.
      The 'knowledge' is a very special variable which will be filled-in with the retrieved chunks.
      All the variables in 'System' should be curly bracketed.`,
      add: 'Add',
      key: 'key',
      optional: 'Optional',
      operation: 'operation',
      model: 'Model',
      modelTip: 'Large language chat model',
      modelMessage: 'Please select!',
      freedom: 'Freedom',
      improvise: 'Improvise',
      precise: 'Precise',
      balance: 'Balance',
      freedomTip: `'Precise' means the LLM will be conservative and answer your question cautiously. 'Improvise' means the you want LLM talk much and freely. 'Balance' is between cautiously and freely.`,
      temperature: 'Temperature',
      temperatureMessage: 'Temperature is required',
      temperatureTip:
        'This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse.',
      topP: 'Top P',
      topPMessage: 'Top P is required',
      topPTip:
        'Also known as “nucleus sampling,” this parameter sets a threshold to select a smaller set of words to sample from. It focuses on the most likely words, cutting off the less probable ones.',
      presencePenalty: 'Presence Penalty',
      presencePenaltyMessage: 'Presence Penalty is required',
      presencePenaltyTip:
        'This discourages the model from repeating the same information by penalizing words that have already appeared in the conversation.',
      frequencyPenalty: 'Frequency Penalty',
      frequencyPenaltyMessage: 'Frequency Penalty is required',
      frequencyPenaltyTip:
        'Similar to the presence penalty, this reduces the model’s tendency to repeat the same words frequently.',
      maxTokens: 'Max Tokens',
      maxTokensMessage: 'Max Tokens is required',
      maxTokensTip:
        'This sets the maximum length of the model’s output, measured in the number of tokens (words or pieces of words).',
      quote: 'Show Quote',
      quoteTip: 'Should the source of the original text be displayed?',
      overview: 'Chat Bot API',
      pv: 'Number of messages',
      uv: 'Active user number',
      speed: 'Token output speed',
      tokens: 'Consume the token number',
      round: 'Session Interaction Number',
      thumbUp: 'customer satisfaction',
      preview: 'Preview',
      embedded: 'Embedded',
      serviceApiEndpoint: 'Service API Endpoint',
      apiKey: 'Api Key',
      apiReference: 'API Documents',
      dateRange: 'Date Range:',
      backendServiceApi: 'Backend service API',
      createNewKey: 'Create new key',
      created: 'Created',
      action: 'Action',
      embedModalTitle: 'Embed into website',
      comingSoon: 'Coming Soon',
      fullScreenTitle: 'Full Embed',
      fullScreenDescription:
        'Embed the following iframe into your website at the desired location',
      partialTitle: 'Partial Embed',
      extensionTitle: 'Chrome Extension',
      tokenError: 'Please create API Token first!',
      searching: 'searching...',
    },
    setting: {
      profile: 'Profile',
      profileDescription: 'Update your photo and personal details here.',
      password: 'Password',
      passwordDescription:
        'Please enter your current password to change your password.',
      model: 'Model Providers',
      modelDescription: 'Set the model parameter and API Key here.',
      team: 'Team',
      logout: 'Log out',
      username: 'Username',
      usernameMessage: 'Please input your username!',
      photo: 'Your photo',
      photoDescription: 'This will be displayed on your profile.',
      colorSchema: 'Color schema',
      colorSchemaMessage: 'Please select your color schema!',
      colorSchemaPlaceholder: 'select your color schema',
      bright: 'Bright',
      dark: 'Dark',
      timezone: 'Timezone',
      timezoneMessage: 'Please input your timezone!',
      timezonePlaceholder: 'select your timezone',
      email: 'Email address',
      emailDescription: 'Once registered, E-mail cannot be changed.',
      currentPassword: 'Current password',
      currentPasswordMessage: 'Please input your password!',
      newPassword: 'New password',
      newPasswordMessage: 'Please input your password!',
      newPasswordDescription:
        'Your new password must be more than 8 characters.',
      confirmPassword: 'Confirm new password',
      confirmPasswordMessage: 'Please confirm your password!',
      confirmPasswordNonMatchMessage:
        'The new password that you entered do not match!',
      cancel: 'Cancel',
      addedModels: 'Added models',
      modelsToBeAdded: 'Models to be added',
      addTheModel: 'Add the model',
      apiKey: 'API-Key',
      apiKeyMessage: 'Please input api key!',
      apiKeyTip:
        'The API key can be obtained by registering the corresponding LLM supplier.',
      showMoreModels: 'Show more models',
      baseUrl: 'Base-Url',
      baseUrlTip:
        'If your API key is from OpenAI, just ignore it. Any other intermediate providers will give this base url with the API key.',
      modify: 'Modify',
      systemModelSettings: 'System Model Settings',
      chatModel: 'Chat model',
      chatModelTip:
        'The default chat LLM all the newly created knowledgebase will use.',
      embeddingModel: 'Embedding model',
      embeddingModelTip:
        'The default embedding model all the newly created knowledgebase will use.',
      img2txtModel: 'Img2txt model',
      img2txtModelTip:
        'The default multi-module model all the newly created knowledgebase will use. It can describe a picture or video.',
      sequence2txtModel: 'Sequence2txt model',
      sequence2txtModelTip:
        'The default ASR model all the newly created knowledgebase will use. Use this model to translate voices to corresponding text.',
      workspace: 'Workspace',
      upgrade: 'Upgrade',
      addLlmTitle: 'Add LLM',
      modelName: 'Model name',
      modelUid: 'Model UID',
      modelNameMessage: 'Please input your model name!',
      modelType: 'Model type',
      modelTypeMessage: 'Please input your model type!',
      addLlmBaseUrl: 'Base url',
      baseUrlNameMessage: 'Please input your base url!',
      vision: 'Does it support Vision?',
      ollamaLink: 'How to integrate {{name}}',
    },
    message: {
      registered: 'Registered!',
      logout: 'logout',
      logged: 'logged!',
      pleaseSelectChunk: 'Please select chunk!',
      modified: 'Modified',
      created: 'Created',
      deleted: 'Deleted',
      renamed: 'Renamed',
      operated: 'Operated',
      updated: 'Updated',
      uploaded: 'Uploaded',
      200: 'The server successfully returns the requested data.',
      201: 'Create or modify data successfully.',
      202: 'A request has been queued in the background (asynchronous task).',
      204: 'Data deleted successfully.',
      400: 'There was an error in the request issued, and the server did not create or modify data.',
      401: 'The user does not have permissions (wrong token, username, password).',
      403: 'The user is authorized, but access is prohibited.',
      404: 'The request was made for a record that does not exist, and the server did not perform the operation.',
      406: 'The requested format is not available.',
      410: 'The requested resource has been permanently deleted and will not be available again.',
      422: 'When creating an object, a validation error occurred.',
      500: 'A server error occurred, please check the server.',
      502: 'Gateway error.',
      503: 'The service is unavailable and the server is temporarily overloaded or undergoing maintenance.',
      504: 'Gateway timeout.',
      requestError: 'Request error',
      networkAnomalyDescription:
        'There is an abnormality in your network and you cannot connect to the server.',
      networkAnomaly: 'network anomaly',
      hint: 'hint',
    },
    fileManager: {
      name: 'Name',
      uploadDate: 'Upload Date',
      knowledgeBase: 'Knowledge Base',
      size: 'Size',
      action: 'Action',
      addToKnowledge: 'Link to Knowledge Base',
      pleaseSelect: 'Please select',
      newFolder: 'New Folder',
      file: 'File',
      uploadFile: 'Upload File',
      directory: 'Directory',
      uploadTitle: 'Click or drag file to this area to upload',
      uploadDescription:
        'Support for a single or bulk upload. Strictly prohibited from uploading company data or other banned files.',
      local: 'Local uploads',
      s3: 'S3 uploads',
      preview: 'Preview',
      fileError: 'File error',
    },
    footer: {
      profile: 'All rights reserved @ React',
    },
    layout: {
      file: 'file',
      knowledge: 'knowledge',
      chat: 'chat',
    },
  },
};
