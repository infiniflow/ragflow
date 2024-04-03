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
      namePlaceholder: 'Please input name',
      next: 'Next',
      create: 'Create',
      edit: 'Edit',
      upload: 'Upload',
      english: 'English',
      chinese: 'Chinese',
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
    },
    header: {
      knowledgeBase: 'Knowledge Base',
      chat: 'Chat',
      register: 'Register',
      signin: 'Sign in',
      home: 'Home',
      setting: '用户设置',
      logout: '登出',
    },
    knowledgeList: {
      welcome: 'Welcome back',
      description: 'Which database are we going to use today?',
      createKnowledgeBase: 'Create knowledge base',
      name: 'Name',
      namePlaceholder: 'Please input name!',
      doc: 'Docs',
    },
    knowledgeDetails: {
      dataset: 'Dataset',
      testing: 'Retrieval testing',
      configuration: 'Configuration',
      name: 'Name',
      namePlaceholder: 'Please input name!',
      doc: 'Docs',
      datasetDescription:
        "Hey, don't forget to adjust the chunk after adding the dataset! 😉",
      addFile: 'Add file',
      searchFiles: 'Search your files',
      localFiles: 'Local files',
      emptyFiles: 'Create empty file',
      chunkNumber: 'Chunk Number',
      uploadDate: 'Upload Date',
      chunkMethod: 'Chunk Method',
      enabled: 'Enabled',
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
      runningStatus0: 'UNSTART',
      runningStatus1: 'Parsing',
      runningStatus2: 'CANCEL',
      runningStatus3: 'SUCCESS',
      runningStatus4: 'FAIL',
      pageRanges: 'Page Ranges:',
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
        'This visual guides is in order to make understanding easier for you.',
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
      qa: `<p><b>EXCEL</b> and <b>CSV/TXT</b> files are supported.</p><p>
      If the file is in excel format, there should be 2 columns question and answer without header.
      And question column is ahead of answer column.
      And it's O.K if it has multiple sheets as long as the columns are rightly composed.</p><p>
    
      If it's in csv format, it should be UTF-8 encoded. Use TAB as delimiter to separate question and answer.</p><p>
    
      <i>All the deformed lines will be ignored.
      Every pair of Q&A will be treated as a chunk.</i></p>`,
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
      enabledSelected: 'Enabled Selected',
      disabledSelected: 'Disabled Selected',
      deleteSelected: 'Delete Selected',
      search: 'Search',
      all: 'All',
      enabled: 'Enabled',
      disabled: 'Disabled',
      keyword: 'Keyword',
      function: 'Function',
      chunkMessage: 'Please input value!',
    },
    chat: {
      chat: 'Chat',
      newChat: 'New chat',
      send: 'Send',
      sendPlaceholder: 'Message Resume Assistant...',
      chatConfiguration: 'Chat Configuration',
      chatConfigurationDescription:
        ' Here, dress up a dedicated assistant for your special knowledge bases! 💕',
      assistantName: 'Assistant name',
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
      systemInitialValue: `你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。
      以下是知识库：
      {knowledge}
      以上是知识库。`,
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
      freedomTip: `'Precise' means the LLM will be conservative and answer your question cautiously. 'Improvise' means the you want LLM talk much and freely. 'Balance' is between cautiously and freely.`,
      temperature: 'Temperature',
      temperatureMessage: 'Temperature is required',
      temperatureTip:
        'This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse.',
      topP: 'Top P',
      topPMessage: 'Top P is required',
      topPTip:
        'This parameter controls the randomness of predictions by the model. A lower temperature makes the model more confident in its responses, while a higher temperature makes it more creative and diverse.',
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
    },
    footer: {
      detail: 'All rights reserved @ React',
    },
    layout: {
      file: 'file',
      knowledge: 'knowledge',
      chat: 'chat',
    },
    setting: {
      btn: 'en',
    },
  },
};
