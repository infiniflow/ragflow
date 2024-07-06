export default {
  translation: {
    common: {
      delete: '删除',
      deleteModalTitle: '确定删除吗?',
      ok: '是',
      cancel: '否',
      total: '总共',
      rename: '重命名',
      name: '名称',
      save: '保存',
      namePlaceholder: '请输入名称',
      next: '下一步',
      create: '创建',
      edit: '编辑',
      upload: '上传',
      english: '英文',
      chinese: '简体中文',
      traditionalChinese: '繁体中文',
      language: '语言',
      languageMessage: '请输入语言',
      languagePlaceholder: '请选择语言',
      copy: '复制',
      copied: '复制成功',
      comingSoon: '即将推出',
      download: '下载',
      close: '关闭',
      preview: '预览',
    },
    login: {
      login: '登录',
      signUp: '注册',
      loginDescription: '很高兴再次见到您！',
      registerDescription: '很高兴您加入！',
      emailLabel: '邮箱',
      emailPlaceholder: '请输入邮箱地址',
      passwordLabel: '密码',
      passwordPlaceholder: '请输入密码',
      rememberMe: '记住我',
      signInTip: '没有帐户？',
      signUpTip: '已经有帐户？',
      nicknameLabel: '名称',
      nicknamePlaceholder: '请输入名称',
      register: '创建账户',
      continue: '继续',
      title: '开始构建您的智能助手',
      description:
        '免费注册以探索顶级 RAG 技术。 创建知识库和人工智能来增强您的业务',
      review: '来自 500 多条评论',
    },
    header: {
      knowledgeBase: '知识库',
      chat: '聊天',
      register: '注册',
      signin: '登录',
      home: '首页',
      setting: '用户设置',
      logout: '登出',
      fileManager: '文件管理',
    },
    knowledgeList: {
      welcome: '欢迎回来',
      description: '今天我们要使用哪个知识库？',
      createKnowledgeBase: '创建知识库',
      name: '名称',
      namePlaceholder: '请输入名称',
      doc: '文档',
      searchKnowledgePlaceholder: '搜索',
    },
    knowledgeDetails: {
      dataset: '数据集',
      testing: '检索测试',
      configuration: '配置',
      files: '文件',
      name: '名称',
      namePlaceholder: '请输入名称',
      doc: '文档',
      datasetDescription: '😉 解析成功后才能问答哦。',
      addFile: '新增文件',
      searchFiles: '搜索文件',
      localFiles: '本地文件',
      emptyFiles: '新建空文件',
      webCrawl: '网页抓取',
      chunkNumber: '分块数',
      uploadDate: '上传日期',
      chunkMethod: '解析方法',
      enabled: '启用',
      disabled: '禁用',
      action: '动作',
      parsingStatus: '解析状态',
      processBeginAt: '流程开始于',
      processDuration: '过程持续时间',
      progressMsg: '进度消息',
      testingDescription: '最后一步！ 成功后，剩下的就交给Infiniflow AI吧。',
      similarityThreshold: '相似度阈值',
      similarityThresholdTip:
        '我们使用混合相似度得分来评估两行文本之间的距离。 它是加权关键词相似度和向量余弦相似度。 如果查询和块之间的相似度小于此阈值，则该块将被过滤掉。',
      vectorSimilarityWeight: '关键字相似度权重',
      vectorSimilarityWeightTip:
        '我们使用混合相似性评分来评估两行文本之间的距离。它是加权关键字相似性和矢量余弦相似性或rerank得分（0〜1）。两个权重的总和为1.0。',
      testText: '测试文本',
      testTextPlaceholder: '请输入您的问题！',
      testingLabel: '测试',
      similarity: '混合相似度',
      termSimilarity: '关键词相似度',
      vectorSimilarity: '向量相似度',
      hits: '命中数',
      view: '看法',
      filesSelected: '选定的文件',
      upload: '上传',
      run: '启动',
      runningStatus0: '未启动',
      runningStatus1: '解析中',
      runningStatus2: '取消',
      runningStatus3: '成功',
      runningStatus4: '失败',
      pageRanges: '页码范围',
      pageRangesTip:
        '页码范围：定义需要解析的页面范围。 不包含在这些范围内的页面将被忽略。',
      fromPlaceholder: '从',
      fromMessage: '缺少起始页码',
      toPlaceholder: '到',
      toMessage: '缺少结束页码（不包含）',
      layoutRecognize: '布局识别',
      layoutRecognizeTip:
        '使用视觉模型进行布局分析，以更好地识别文档结构，找到标题、文本块、图像和表格的位置。 如果没有此功能，则只能获取 PDF 的纯文本。',
      taskPageSize: '任务页面大小',
      taskPageSizeMessage: '请输入您的任务页面大小！',
      taskPageSizeTip: `如果使用布局识别，PDF 文件将被分成连续的组。 布局分析将在组之间并行执行，以提高处理速度。 “任务页面大小”决定组的大小。 页面大小越大，将页面之间的连续文本分割成不同块的机会就越低。`,
      addPage: '新增页面',
      greaterThan: '当前值必须大于起始值！',
      greaterThanPrevious: '当前值必须大于之前的值！',
      selectFiles: '选择文件',
      changeSpecificCategory: '更改特定类别',
      uploadTitle: '点击或拖拽文件至此区域即可上传',
      uploadDescription:
        '支持单次或批量上传。 严禁上传公司数据或其他违禁文件。',
      chunk: '解析块',
      bulk: '批量',
      cancel: '取消',
      rerankModel: 'Rerank模型',
      rerankPlaceholder: '请选择',
      rerankTip: `如果是空的。它使用查询和块的嵌入来构成矢量余弦相似性。否则，它使用rerank评分代替矢量余弦相似性。`,
      topK: 'Top-K',
      topKTip: `K块将被送入Rerank型号。`,
    },
    knowledgeConfiguration: {
      titleDescription: '在这里更新您的知识库详细信息，尤其是解析方法。',
      name: '知识库名称',
      photo: '知识库图片',
      description: '描述',
      language: '语言',
      languageMessage: '请输入语言',
      languagePlaceholder: '请输入语言',
      permissions: '权限',
      embeddingModel: '嵌入模型',
      chunkTokenNumber: '块Token数',
      chunkTokenNumberMessage: '块Token数是必填项',
      embeddingModelTip:
        '用于嵌入块的嵌入模型。 一旦知识库有了块，它就无法更改。 如果你想改变它，你需要删除所有的块。',
      permissionsTip: '如果权限是“团队”，则所有团队成员都可以操作知识库。',
      chunkTokenNumberTip: '它大致确定了一个块的Token数量。',
      chunkMethod: '解析方法',
      chunkMethodTip: '说明位于右侧。',
      upload: '上传',
      english: '英文',
      chinese: '中文',
      embeddingModelPlaceholder: '请选择嵌入模型',
      chunkMethodPlaceholder: '请选择分块方法',
      save: '保存',
      me: '只有我',
      team: '团队',
      cancel: '取消',
      methodTitle: '分块方法说明',
      methodExamples: '示例',
      methodExamplesDescription: '提出以下屏幕截图以促进理解。',
      dialogueExamplesTitle: '对话示例',
      methodEmpty: '这将显示知识库类别的可视化解释',
      book: `<p>支持的文件格式为<b>DOCX</b>、<b>PDF</b>、<b>TXT</b>。</p><p>
      由于一本书很长，并不是所有部分都有用，如果是 PDF，
      请为每本书设置<i>页面范围</i>，以消除负面影响并节省分析计算时间。</p>`,
      laws: `<p>支持的文件格式为<b>DOCX</b>、<b>PDF</b>、<b>TXT</b>。</p><p>
      法律文件有非常严格的书写格式。 我们使用文本特征来检测分割点。
      </p><p>
      chunk的粒度与'ARTICLE'一致，所有上层文本都会包含在chunk中。
      </p>`,
      manual: `<p>仅支持<b>PDF</b>。</p><p>
      我们假设手册具有分层部分结构。 我们使用最低的部分标题作为对文档进行切片的枢轴。
      因此，同一部分中的图和表不会被分割，并且块大小可能会很大。
      </p>`,
      naive: `<p>支持的文件格式为<b>DOCX、EXCEL、PPT、IMAGE、PDF、TXT</b>。</p>
      <p>此方法将简单的方法应用于块文件：</p>
      <p>
      <li>系统将使用视觉检测模型将连续文本分割成多个片段。</li>
      <li>接下来，这些连续的片段被合并成Token数不超过“Token数”的块。</li></p>`,
      paper: `<p>仅支持<b>PDF</b>文件。</p><p>
      如果我们的模型运行良好，论文将按其部分进行切片，例如<i>摘要、1.1、1.2</i>等。</p><p>
      这样做的好处是LLM可以更好的概括论文中相关章节的内容，
      产生更全面的答案，帮助读者更好地理解论文。
      缺点是它增加了 LLM 对话的背景并增加了计算成本，
      所以在对话过程中，你可以考虑减少‘<b>topN</b>’的设置。</p>`,
      presentation: `<p>支持的文件格式为<b>PDF</b>、<b>PPTX</b>。</p><p>
      每个页面都将被视为一个块。 并且每个页面的缩略图都会被存储。</p><p>
      <i>您上传的所有PPT文件都会使用此方法自动分块，无需为每个PPT文件进行设置。</i></p>`,
      qa: ` <p>
      此块方法支持<b> excel </b>和<b> csv/txt </b>文件格式。
    </p>
    <li>
      如果文件以<b> excel </b>格式，则应由两个列组成
      没有标题：一个提出问题，另一个用于答案，
      答案列之前的问题列。多张纸是
      只要列正确结构，就可以接受。
    </li>
    <li>
      如果文件以<b> csv/txt </b>格式为
      用作分开问题和答案的定界符。
    </li>
    <p>
      <i>
        未能遵循上述规则的文本行将被忽略，并且
        每个问答对将被认为是一个独特的部分。
      </i>
    </p>`,
      resume: `<p>支持的文件格式为<b>DOCX</b>、<b>PDF</b>、<b>TXT</b>。
      </p><p>
      简历有多种格式，就像一个人的个性一样，但我们经常必须将它们组织成结构化数据，以便于搜索。
      </p><p>
      我们不是将简历分块，而是将简历解析为结构化数据。 作为HR，你可以扔掉所有的简历，
      您只需与<i>'RAGFlow'</i>交谈即可列出所有符合资格的候选人。
      </p>
        `,
      table: `支持<p><b>EXCEL</b>和<b>CSV/TXT</b>格式文件。</p><p>
      以下是一些提示：
      <ul>
    <li>对于 csv 或 txt 文件，列之间的分隔符为 <em><b>TAB</b></em>。</li>
    <li>第一行必须是列标题。</li>
    <li>列标题必须是有意义的术语，以便我们的大语言模型能够理解。
    列举一些同义词时最好使用斜杠<i>'/'</i>来分隔，甚至更好
    使用方括号枚举值，例如 <i>'gender/sex(male,female)'</i>.<p>
    以下是标题的一些示例：<ol>
        <li>供应商/供货商<b>'TAB'</b>颜色（黄色、红色、棕色）<b>'TAB'</b>性别（男、女）<b>'TAB'</ b>尺码（M、L、XL、XXL）</li>
        <li>姓名/名字<b>'TAB'</b>电话/手机/微信<b>'TAB'</b>最高学历（高中，职高，硕士，本科，博士，初中，中技，中 专，专科，专升本，MPA，MBA，EMBA）</li>
        </ol>
        </p>
    </li>
    <li>表中的每一行都将被视为一个块。</li>
    </ul>`,
      picture: `
      <p>支持图像文件。 视频即将推出。</p><p>
      如果图片中有文字，则应用 OCR 提取文字作为其文字描述。
      </p><p>
      如果OCR提取的文本不够，可以使用视觉LLM来获取描述。
      </p>`,
      one: `
      <p>支持的文件格式为<b>DOCX、EXCEL、PDF、TXT</b>。
      </p><p>
      对于一个文档，它将被视为一个完整的块，根本不会被分割。
      </p><p>
      如果你要总结的东西需要一篇文章的全部上下文，并且所选LLM的上下文长度覆盖了文档长度，你可以尝试这种方法。
      </p>`,
      useRaptor: '使用召回增强RAPTOR策略',
      useRaptorTip: '请参考 https://huggingface.co/papers/2401.18059',
      prompt: '提示词',
      promptMessage: '提示词是必填项',
      promptText: `请总结以下段落。 小心数字，不要编造。 段落如下：
      {cluster_content}
以上就是你需要总结的内容。`,
      maxToken: '最大token数',
      maxTokenMessage: '最大token数是必填项',
      threshold: '阈值',
      thresholdMessage: '阈值是必填项',
      maxCluster: '最大聚类数',
      maxClusterMessage: '最大聚类数是必填项',
      randomSeed: '随机种子',
      randomSeedMessage: '随机种子是必填项',
      promptTip: 'LLM提示用于总结。',
      maxTokenTip: '用于汇总的最大token数。',
      thresholdTip: '阈值越大，聚类越少。',
      maxClusterTip: '最大聚类数。',
    },
    chunk: {
      chunk: '解析块',
      bulk: '批量',
      selectAll: '选择所有',
      enabledSelected: '启用选定的',
      disabledSelected: '禁用选定的',
      deleteSelected: '删除选定的',
      search: '搜索',
      all: '所有',
      enabled: '启用',
      disabled: '禁用的',
      keyword: '关键词',
      function: '函数',
      chunkMessage: '请输入值！',
      full: '全文',
      ellipse: '省略',
    },
    chat: {
      createAssistant: '新建助理',
      assistantSetting: '助理设置',
      promptEngine: '提示引擎',
      modelSetting: '模型设置',
      chat: '聊天',
      newChat: '新建聊天',
      send: '发送',
      sendPlaceholder: '消息概要助手...',
      chatConfiguration: '聊天配置',
      chatConfigurationDescription: '在这里，为你的专业知识库装扮专属助手！ 💕',
      assistantName: '助理姓名',
      assistantNameMessage: '助理姓名是必填项',
      namePlaceholder: '例如 贾维斯简历',
      assistantAvatar: '助理头像',
      language: '语言',
      emptyResponse: '空回复',
      emptyResponseTip: `如果在知识库中没有检索到用户的问题，它将使用它作为答案。 如果您希望 LLM 在未检索到任何内容时提出自己的意见，请将此留空。`,
      setAnOpener: '设置开场白',
      setAnOpenerInitial: `你好！ 我是你的助理，有什么可以帮到你的吗？`,
      setAnOpenerTip: '您想如何欢迎您的客户？',
      knowledgeBases: '知识库',
      knowledgeBasesMessage: '请选择',
      knowledgeBasesTip: '选择关联的知识库。',
      system: '系统',
      systemInitialValue: `你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。
        以下是知识库：
        {knowledge}
        以上是知识库。`,
      systemMessage: '请输入',
      systemTip:
        '当LLM回答问题时，你需要LLM遵循的说明，比如角色设计、答案长度和答案语言等。',
      topN: 'Top N',
      topNTip: `并非所有相似度得分高于“相似度阈值”的块都会被提供给大语言模型。 LLM 只能看到这些“Top N”块。`,
      variable: '变量',
      variableTip: `如果您使用对话 API，变量可能会帮助您使用不同的策略与客户聊天。
      这些变量用于填写提示中的“系统”部分，以便给LLM一个提示。
      “知识”是一个非常特殊的变量，它将用检索到的块填充。
      “System”中的所有变量都应该用大括号括起来。`,
      add: '新增',
      key: '关键字',
      optional: '可选的',
      operation: '操作',
      model: '模型',
      modelTip: '大语言聊天模型',
      modelMessage: '请选择',
      freedom: '自由',
      improvise: '即兴创作',
      precise: '精确',
      balance: '平衡',
      freedomTip: `“精确”意味着大语言模型会保守并谨慎地回答你的问题。 “即兴发挥”意味着你希望大语言模型能够自由地畅所欲言。 “平衡”是谨慎与自由之间的平衡。`,
      temperature: '温度',
      temperatureMessage: '温度是必填项',
      temperatureTip:
        '该参数控制模型预测的随机性。 较低的温度使模型对其响应更有信心，而较高的温度则使其更具创造性和多样性。',
      topP: 'Top P',
      topPMessage: 'Top P 是必填项',
      topPTip:
        '该参数也称为“核心采样”，它设置一个阈值来选择较小的单词集进行采样。 它专注于最可能的单词，剔除不太可能的单词。',
      presencePenalty: '出席处罚',
      presencePenaltyMessage: '出席处罚是必填项',
      presencePenaltyTip:
        '这会通过惩罚对话中已经出现的单词来阻止模型重复相同的信息。',
      frequencyPenalty: '频率惩罚',
      frequencyPenaltyMessage: '频率惩罚是必填项',
      frequencyPenaltyTip:
        '与存在惩罚类似，这减少了模型频繁重复相同单词的倾向。',
      maxTokens: '最大token数',
      maxTokensMessage: '最大token数是必填项',
      maxTokensTip:
        '这设置了模型输出的最大长度，以标记（单词或单词片段）的数量来衡量。',
      quote: '显示引文',
      quoteTip: '是否应该显示原文出处？',
      selfRag: 'Self-RAG',
      selfRagTip: '请参考: https://huggingface.co/papers/2310.11511',
      overview: '聊天 API',
      pv: '消息数',
      uv: '活跃用户数',
      speed: 'Token 输出速度',
      tokens: '消耗Token数',
      round: '会话互动数',
      thumbUp: '用户满意度',
      preview: '预览',
      embedded: '嵌入',
      serviceApiEndpoint: '服务API端点',
      apiKey: 'API 键',
      apiReference: 'API 文档',
      dateRange: '日期范围：',
      backendServiceApi: '后端服务 API',
      createNewKey: '创建新密钥',
      created: '创建于',
      action: '操作',
      embedModalTitle: '嵌入网站',
      comingSoon: '即将推出',
      fullScreenTitle: '全屏嵌入',
      fullScreenDescription: '将以下iframe嵌入您的网站处于所需位置',
      partialTitle: '部分嵌入',
      extensionTitle: 'Chrome 插件',
      tokenError: '请先创建 Api Token!',
      searching: '搜索中',
    },
    setting: {
      profile: '概要',
      profileDescription: '在此更新您的照片和个人详细信息。',
      password: '密码',
      passwordDescription: '请输入您当前的密码以更改您的密码。',
      model: '模型提供商',
      modelDescription: '在此设置模型参数和 API Key。',
      team: '团队',
      system: '系统',
      logout: '登出',
      username: '用户名',
      usernameMessage: '请输入用户名',
      photo: '头像',
      photoDescription: '这将显示在您的个人资料上。',
      colorSchema: '主题',
      colorSchemaMessage: '请选择您的主题！',
      colorSchemaPlaceholder: '请选择您的主题！',
      bright: '明亮',
      dark: '暗色',
      timezone: '时区',
      timezoneMessage: '请选择时区',
      timezonePlaceholder: '请选择时区',
      email: '邮箱地址',
      emailDescription: '一旦注册，电子邮件将无法更改。',
      currentPassword: '当前密码',
      currentPasswordMessage: '请输入当前密码',
      newPassword: '新密码',
      newPasswordMessage: '请输入新密码',
      newPasswordDescription: '您的新密码必须超过 8 个字符。',
      confirmPassword: '确认新密码',
      confirmPasswordMessage: '请确认新密码',
      confirmPasswordNonMatchMessage: '您输入的新密码不匹配！',
      cancel: '取消',
      addedModels: '添加了的模型',
      modelsToBeAdded: '待添加的模型',
      addTheModel: '添加模型',
      apiKey: 'API-Key',
      apiKeyMessage: '请输入 api key!',
      apiKeyTip: 'API key可以通过注册相应的LLM供应商来获取。',
      showMoreModels: '展示更多模型',
      baseUrl: 'Base-Url',
      baseUrlTip:
        '如果您的 API 密钥来自 OpenAI，请忽略它。 任何其他中间提供商都会提供带有 API 密钥的基本 URL。',
      modify: '修改',
      systemModelSettings: '系统模型设置',
      chatModel: '聊天模型',
      chatModelTip: '所有新创建的知识库都会使用默认的聊天LLM。',
      embeddingModel: '嵌入模型',
      embeddingModelTip: '所有新创建的知识库都将使用的默认嵌入模型。',
      img2txtModel: 'Img2txt模型',
      img2txtModelTip:
        '所有新创建的知识库都将使用默认的多模块模型。 它可以描述图片或视频。',
      sequence2txtModel: 'Sequence2txt模型',
      sequence2txtModelTip:
        '所有新创建的知识库都将使用默认的 ASR 模型。 使用此模型将语音翻译为相应的文本。',
      rerankModel: 'Rerank模型',
      rerankModelTip: `默认的重读模型用于用户问题检索到重读块。`,
      workspace: '工作空间',
      upgrade: '升级',
      addLlmTitle: '添加 LLM',
      modelName: '模型名称',
      modelUid: '模型UID',
      modelType: '模型类型',
      addLlmBaseUrl: '基础 Url',
      vision: '是否支持 Vision',
      modelNameMessage: '请输入模型名称！',
      modelTypeMessage: '请输入模型类型！',
      baseUrlNameMessage: '请输入基础 Url！',
      ollamaLink: '如何集成 {{name}}',
      volcModelNameMessage: '请输入模型名称！格式：{"模型名称":"EndpointID"}',
      addVolcEngineAK: '火山 ACCESS_KEY',
      volcAKMessage: '请输入VOLC_ACCESS_KEY',
      addVolcEngineSK: '火山 SECRET_KEY',
      volcSKMessage: '请输入VOLC_SECRET_KEY',
    },
    message: {
      registered: '注册成功',
      logout: '登出成功',
      logged: '登录成功',
      pleaseSelectChunk: '请选择解析块',
      modified: '更新成功',
      created: '创建成功',
      deleted: '删除成功',
      renamed: '重命名成功',
      operated: '操作成功',
      updated: '更新成功',
      uploaded: '上传成功',
      200: '服务器成功返回请求的数据。',
      201: '新建或修改数据成功。',
      202: '一个请求已经进入后台排队（异步任务）。',
      204: '删除数据成功。',
      400: '发出的请求有错误，服务器没有进行新建或修改数据的操作。',
      401: '用户没有权限（Token、用户名、密码错误）。',
      403: '用户得到授权，但是访问是被禁止的。',
      404: '发出的请求针对的是不存在的记录，服务器没有进行操作。',
      406: '请求的格式不可得。',
      410: '请求的资源被永久删除，且不会再得到的。',
      422: '当创建一个对象时，发生一个验证错误。',
      500: '服务器发生错误，请检查服务器。',
      502: '网关错误。',
      503: '服务不可用，服务器暂时过载或维护。',
      504: '网关超时。',
      requestError: '请求错误',
      networkAnomalyDescription: '您的网络发生异常，无法连接服务器',
      networkAnomaly: '网络异常',
      hint: '提示',
    },
    fileManager: {
      name: '名称',
      uploadDate: '上传日期',
      knowledgeBase: '知识库',
      size: '大小',
      action: '操作',
      addToKnowledge: '链接知识库',
      pleaseSelect: '请选择',
      newFolder: '新建文件夹',
      uploadFile: '上传文件',
      uploadTitle: '点击或拖拽文件至此区域即可上传',
      uploadDescription:
        '支持单次或批量上传。 严禁上传公司数据或其他违禁文件。',
      file: '文件',
      directory: '文件夹',
      local: '本地上传',
      s3: 'S3 上传',
      preview: '预览',
      fileError: '文件错误',
    },
    flow: {
      flow: '工作流',
      cite: '引用',
      citeTip: 'citeTip',
      name: '名称',
      nameMessage: '请输入名称',
      description: '描述',
      examples: '示例',
      to: '到',
      message: '消息',
      messagePlaceholder: '消息',
      messageMsg: '请输入消息或删除此字段。',
      addField: '新增字段',
      loop: '环',
      createFlow: 'Create a workflow',
      yes: 'Yes',
      no: 'No',
      key: 'key',
      componentId: 'component id',
      add: 'Add',
      operation: 'operation',
      beginDescription: 'This is where the flow begin',
      answerDescription: `This component is used as an interface between bot and human. It receives input of user and display the result of the computation of the bot.`,
      retrievalDescription: `This component is for the process of retrieving relevent information from knowledge base. So, knowledgebases should be selected. If there's nothing retrieved, the 'Empty response' will be returned.`,
      generateDescription: `This component is used to call LLM to generate text. Be careful about the prompt setting.`,
      categorizeDescription: `This component is used to categorize text. Please specify the name, description and examples of the category. Every single category leads to different downstream components.`,
      relevantDescription: `This component is used to judge if the output of upstream is relevent to user's latest question. 'Yes' represents that they're relevant. 'No' represents they're irrelevant.`,
      rewriteQuestionDescription: `This component is used to refine user's quesion. Typically, when a user's original question can't retrieve relevant information from knowledge base, this component help you change the question into a proper one which might be more consistant with the expressions in knowledge base. Only 'Retrieval' can be its downstreams.`,
      messageDescription:
        'This component is used to send user static information.',
      keywordDescription:
        'This component is used to send user static information.',
      promptText: `Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:
        {input}
  The above is the content you need to summarize.`,
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
