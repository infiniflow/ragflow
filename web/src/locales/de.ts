export default {
  translation: {
    common: {
      confirm: 'Bestätigen',
      back: 'Zurück',
      noResults: 'Keine Ergebnisse gefunden',
      selectPlaceholder: 'Wert auswählen',
      selectAll: 'Alle auswählen',
      delete: 'Löschen',
      deleteModalTitle:
        'Sind Sie sicher, dass Sie diesen Eintrag löschen möchten?',
      deleteThem: 'Sind Sie sicher, dass Sie diese löschen möchten?',
      ok: 'Ja',
      cancel: 'Abbrechen',
      yes: 'Ja',
      no: 'Nein',
      total: 'Gesamt',
      rename: 'Umbenennen',
      name: 'Name',
      save: 'Speichern',
      namePlaceholder: 'Bitte Namen eingeben',
      next: 'Weiter',
      create: 'Erstellen',
      edit: 'Bearbeiten',
      upload: 'Hochladen',
      english: 'Englisch',
      portugueseBr: 'Portugiesisch (Brasilien)',
      chinese: 'Vereinfachtes Chinesisch',
      traditionalChinese: 'Traditionelles Chinesisch',
      russian: 'Russisch',
      bulgarian: 'Bulgarisch',
      arabic: 'Arabisch',
      german: 'Deutsch',
      language: 'Sprache',
      languageMessage: 'Bitte geben Sie Ihre Sprache ein!',
      languagePlaceholder: 'Wählen Sie Ihre Sprache',
      copy: 'Kopieren',
      copied: 'Kopiert',
      comingSoon: 'Demnächst verfügbar',
      download: 'Herunterladen',
      close: 'Schließen',
      preview: 'Vorschau',
      move: 'Verschieben',
      warn: 'Warnung',
      action: 'Aktion',
      s: 'S',
      pleaseSelect: 'Bitte auswählen',
      pleaseInput: 'Bitte eingeben',
      submit: 'Absenden',
      clear: 'Leeren',
      embedIntoSite: 'In Webseite einbetten',
      openInNewTab: 'Chat in neuem Tab',
      previousPage: 'Zurück',
      nextPage: 'Weiter',
      add: 'Hinzufügen',
      remove: 'Entfernen',
      search: 'Suchen',
      noDataFound: 'Keine Daten gefunden.',
      noData: 'Keine Daten verfügbar',
      promptPlaceholder:
        'Bitte eingeben oder / verwenden, um Variablen schnell einzufügen.',
      mcp: {
        namePlaceholder: 'Mein MCP-Server',
        nameRequired:
          'Muss 1–64 Zeichen lang sein und darf nur Buchstaben, Zahlen, Bindestriche und Unterstriche enthalten.',
        urlPlaceholder: 'https://api.example.com/v1/mcp',
        tokenPlaceholder: 'z.B. eyJhbGciOiJIUzI1Ni...',
      },
      selected: 'Ausgewählt',
      seeAll: 'Alle anzeigen',
    },
    login: {
      loginTitle: 'Melden Sie sich bei Ihrem Konto an',
      signUpTitle: 'Ein Konto erstellen',
      login: 'Anmelden',
      signUp: 'Registrieren',
      loginDescription: 'Wir freuen uns, Sie wiederzusehen!',
      registerDescription: 'Schön, Sie an Bord zu haben!',
      emailLabel: 'E-Mail',
      emailPlaceholder: 'Bitte E-Mail eingeben',
      passwordLabel: 'Passwort',
      passwordPlaceholder: 'Bitte Passwort eingeben',
      rememberMe: 'Angemeldet bleiben',
      signInTip: 'Noch kein Konto?',
      signUpTip: 'Bereits ein Konto?',
      nicknameLabel: 'Spitzname',
      nicknamePlaceholder: 'Bitte Spitznamen eingeben',
      register: 'Konto erstellen',
      continue: 'Fortfahren',
      title: 'Beginnen Sie mit dem Aufbau Ihrer intelligenten Assistenten.',
      start: 'Lass uns anfangen',
      description:
        'Registrieren Sie sich kostenlos, um führende RAG-Technologie zu erkunden. Erstellen Sie Wissensdatenbanken und KIs, um Ihr Unternehmen zu stärken.',
      review: 'von über 500 Bewertungen',
    },
    header: {
      knowledgeBase: 'Wissensdatenbank',
      chat: 'Chat',
      register: 'Registrieren',
      signin: 'Anmelden',
      home: 'Startseite',
      setting: 'Benutzereinstellungen',
      logout: 'Abmelden',
      fileManager: 'Dateiverwaltung',
      flow: 'Agent',
      search: 'Suche',
      welcome: 'Willkommen bei',
      dataset: 'Datensatz',
      memories: 'Gedächtnis',
      Memories: 'Gedächtnis',
    },
    memories: {
      llmTooltip:
        'Analysiert Gesprächsinhalte, extrahiert Schlüsselinformationen und generiert strukturierte Gedächtniszusammenfassungen.',
      embeddingModelTooltip:
        'Konvertiert Text in numerische Vektoren für die Suche nach Bedeutungsähnlichkeit und den Gedächtnisabruf.',
      embeddingModelError:
        'Speichertyp ist erforderlich und "raw" kann nicht gelöscht werden.',
      memoryTypeTooltip: `Raw: Der rohe Dialoginhalt zwischen Benutzer und Agent (Standardmäßig erforderlich).
Semantisches Gedächtnis: Allgemeines Wissen und Fakten über den Benutzer und die Welt.
Episodisches Gedächtnis: Zeitgestempelte Aufzeichnungen spezifischer Ereignisse und Erfahrungen.
Prozedurales Gedächtnis: Erlernte Fähigkeiten, Gewohnheiten und automatisierte Abläufe.`,
      raw: 'raw',
      semantic: 'semantisch',
      episodic: 'episodisch',
      procedural: 'prozedural',
      editName: 'Namen bearbeiten',
      memory: 'Gedächtnis',
      createMemory: 'Gedächtnis erstellen',
      name: 'Name',
      memoryNamePlaceholder: 'Gedächtnisname',
      memoryType: 'Gedächtnistyp',
      embeddingModel: 'Embedding-Modell',
      selectModel: 'Modell auswählen',
      llm: 'LLM',
      delMemoryWarn:
        'Nach dem Löschen werden alle Nachrichten in diesem Gedächtnis gelöscht und können von Agenten nicht mehr abgerufen werden.',
    },
    memory: {
      messages: {
        forget: 'Vergessen',
        forgetMessageTip: 'Sind Sie sicher, dass Sie vergessen möchten?',
        messageDescription:
          'Der Gedächtnisabruf wird mit Ähnlichkeitsschwellenwert, Schlüsselwortähnlichkeitsgewicht und Top N aus den erweiterten Einstellungen konfiguriert.',
        copied: 'Kopiert!',
        contentEmbed: 'Inhaltseinbettung',
        content: 'Inhalt',
        delMessageWarn:
          'Nach dem Vergessen wird diese Nachricht nicht mehr von Agenten abgerufen.',
        forgetMessage: 'Nachricht vergessen',
        sessionId: 'Sitzungs-ID',
        agent: 'Agent',
        type: 'Typ',
        validDate: 'Gültigkeitsdatum',
        forgetAt: 'Vergessen am',
        source: 'Quelle',
        enable: 'Aktivieren',
        action: 'Aktion',
      },
      config: {
        memorySizeTooltip: `Berücksichtigt den Inhalt jeder Nachricht + deren Einbettungsvektor (≈ Inhalt + Dimensionen × 8 Bytes).
Beispiel: Eine 1 KB Nachricht mit 1024-dim Einbettung verwendet ~9 KB. Das Standardlimit von 5 MB fasst ~500 solcher Nachrichten.`,
        avatar: 'Avatar',
        description: 'Beschreibung',
        memorySize: 'Gedächtnisgröße',
        advancedSettings: 'Erweiterte Einstellungen',
        permission: 'Berechtigung',
        onlyMe: 'Nur ich',
        team: 'Team',
        storageType: 'Speichertyp',
        storageTypePlaceholder: 'Bitte Speichertyp auswählen',
        forgetPolicy: 'Vergessensrichtlinie',
        temperature: 'Temperatur',
        systemPrompt: 'System-Prompt',
        systemPromptPlaceholder: 'Bitte System-Prompt eingeben',
        userPrompt: 'Benutzer-Prompt',
        userPromptPlaceholder: 'Bitte Benutzer-Prompt eingeben',
      },
      sideBar: {
        messages: 'Nachrichten',
        configuration: 'Konfiguration',
      },
    },
    knowledgeList: {
      welcome: 'Willkommen zurück',
      description: 'Welche Wissensdatenbanken möchten Sie heute nutzen?',
      createKnowledgeBase: 'Wissensdatenbank erstellen',
      name: 'Name',
      namePlaceholder: 'Bitte Namen eingeben!',
      doc: 'Dokumente',
      searchKnowledgePlaceholder: 'Suchen',
      noMoreData: `Das war's. Nichts mehr zu sehen.`,
      parserRequired: 'Chunk-Methode ist erforderlich',
    },
    knowledgeDetails: {
      metadata: {
        type: 'Typ',
        fieldNameInvalid:
          'Feldname darf nur Buchstaben oder Unterstriche enthalten.',
        builtIn: 'Eingebaut',
        generation: 'Generierung',
        toMetadataSetting: 'Generierungseinstellungen',
        toMetadataSettingTip: 'Auto-Metadaten in der Konfiguration festlegen.',
        descriptionTip:
          'Geben Sie Beschreibungen oder Beispiele an, um das LLM beim Extrahieren von Werten für dieses Feld zu unterstützen. Wenn leer gelassen, wird der Feldname verwendet.',
        restrictDefinedValuesTip:
          'Enum-Modus: Beschränkt die LLM-Extraktion darauf, nur voreingestellte Werte abzugleichen. Definieren Sie Werte unten.',
        valueExists:
          'Wert existiert bereits. Bestätigen Sie, um Duplikate zusammenzuführen und alle zugehörigen Dateien zu kombinieren.',
        fieldNameExists:
          'Feld existiert bereits. Bestätigen Sie, um Duplikate zusammenzuführen und alle zugehörigen Dateien zu kombinieren.',
        valueSingleExists:
          'Wert existiert bereits. Bestätigen Sie, um Duplikate zusammenzuführen.',
        fieldSingleNameExists:
          'Feldname existiert bereits. Bestätigen Sie, um Duplikate zusammenzuführen.',
        fieldExists: 'Feld existiert bereits.',
        fieldSetting: 'Feldeinstellungen',
        deleteWarn:
          'Dieses {{field}} wird aus allen zugehörigen Dateien entfernt',
        deleteManageFieldAllWarn:
          'Dieses Feld und alle zugehörigen Werte werden aus allen zugehörigen Dateien gelöscht.',
        deleteManageValueAllWarn:
          'Dieser Wert wird aus allen zugehörigen Dateien gelöscht.',
        deleteManageFieldSingleWarn:
          'Dieses Feld und alle zugehörigen Werte werden aus diesen Dateien gelöscht.',
        deleteManageValueSingleWarn:
          'Dieser Wert wird aus diesen Dateien gelöscht.',
        deleteSettingFieldWarn: `Dieses Feld wird gelöscht; vorhandene Metadaten sind davon nicht betroffen.`,
        deleteSettingValueWarn: `Dieser Wert wird gelöscht; vorhandene Metadaten sind davon nicht betroffen.`,
        changesAffectNewParses: 'Änderungen betreffen nur neue Analysen.',
        editMetadataForDataset: 'Metadaten anzeigen und bearbeiten für ',
        restrictDefinedValues: 'Auf definierte Werte beschränken',
        metadataGenerationSettings: 'Einstellungen zur Metadatengenerierung',
        manageMetadataForDataset: 'Metadaten für diesen Datensatz verwalten',
        manageMetadata: 'Metadaten verwalten',
        metadata: 'Metadaten',
        values: 'Werte',
        value: 'Wert',
        action: 'Aktion',
        field: 'Feld',
        description: 'Beschreibung',
        fieldName: 'Feldname',
        editMetadata: 'Metadaten bearbeiten',
      },
      redoAll: 'Vorhandene Chunks löschen',
      applyAutoMetadataSettings:
        'Globale Auto-Metadaten-Einstellungen anwenden',
      parseFileTip: 'Sind Sie sicher, dass Sie die Datei analysieren möchten?',
      parseFile: 'Datei analysieren',
      emptyMetadata: 'Keine Metadaten',
      metadataField: 'Metadatenfeld',
      systemAttribute: 'Systemattribut',
      localUpload: 'Lokaler Upload',
      fileSize: 'Dateigröße',
      fileType: 'Dateityp',
      uploadedBy: 'Hochgeladen von',
      notGenerated: 'Nicht generiert',
      generatedOn: 'Generiert am ',
      subbarFiles: 'Dateien',
      generateKnowledgeGraph:
        'Dies extrahiert Entitäten und Beziehungen aus allen Ihren Dokumenten in diesem Datensatz. Der Vorgang kann eine Weile dauern.',
      generateRaptor:
        'Führt rekursives Clustering und Zusammenfassen von Dokument-Chunks durch, um eine hierarchische Baumstruktur aufzubauen, die einen kontextbezogeneren Abruf über lange Dokumente hinweg ermöglicht.',
      generate: 'Generieren',
      raptor: 'RAPTOR',
      processingType: 'Verarbeitungstyp',
      dataPipeline: 'Wechseln oder konfigurieren Sie die Ingestion-Pipeline.',
      operations: 'Operationen',
      taskId: 'Aufgaben-ID',
      duration: 'Dauer',
      details: 'Details',
      status: 'Status',
      task: 'Aufgabe',
      startDate: 'Startdatum',
      source: 'Quelle',
      fileName: 'Dateiname',
      datasetLogs: 'Datensatz',
      fileLogs: 'Datei',
      overview: 'Logs',
      success: 'Erfolg',
      failed: 'Fehlgeschlagen',
      completed: 'Abgeschlossen',
      datasetLog: 'Datensatz-Log',
      created: 'Erstellt',
      learnMore: 'Einführung in die integrierte Pipeline',
      general: 'Allgemein',
      chunkMethodTab: 'Chunk-Methode',
      testResults: 'Ergebnisse',
      testSetting: 'Einstellung',
      retrievalTesting: 'Abruftest',
      retrievalTestingDescription:
        'Führen Sie einen Abruftest durch, um zu prüfen, ob RAGFlow die beabsichtigten Inhalte für das LLM wiederherstellen kann.',
      Parse: 'Analysieren',
      dataset: 'Datensatz',
      testing: 'Abruftest',
      files: 'Dateien',
      configuration: 'Konfiguration',
      knowledgeGraph: 'Wissensgraph',
      name: 'Name',
      namePlaceholder: 'Bitte Namen eingeben!',
      doc: 'Dokumente',
      datasetDescription:
        '😉 Bitte warten Sie, bis die Analyse Ihrer Datei abgeschlossen ist, bevor Sie einen KI-gestützten Chat starten.',
      addFile: 'Datei hinzufügen',
      searchFiles: 'Durchsuchen Sie Ihre Dateien',
      localFiles: 'Lokale Dateien',
      emptyFiles: 'Leere Datei erstellen',
      webCrawl: 'Web-Crawling',
      chunkNumber: 'Chunk-Anzahl',
      uploadDate: 'Hochladedatum',
      chunkMethod: 'Chunk-Methode',
      enabled: 'Aktiviert',
      disabled: 'Deaktiviert',
      action: 'Aktion',
      parsingStatus: 'Analysestatus',
      parsingStatusTip:
        'Die Verarbeitungszeit für Dokumente variiert je nach mehreren Faktoren. Das Aktivieren von Funktionen wie Knowledge Graph, RAPTOR, automatischer Frage- oder Schlüsselwort-Extraktion verlängert die Bearbeitungszeit deutlich. Wenn der Fortschrittsbalken stehen bleibt, konsultieren Sie bitte diese beiden FAQs: https://ragflow.io/docs/dev/faq#why-does-my-document-parsing-stall-at-under-one-percent.',
      processBeginAt: 'Beginn',
      processDuration: 'Dauer',
      progressMsg: 'Fortschritt',
      noTestResultsForRuned:
        'Keine relevanten Ergebnisse gefunden. Versuchen Sie, Ihre Abfrage oder Parameter anzupassen.',
      noTestResultsForNotRuned:
        'Es wurde noch kein Test durchgeführt. Ergebnisse werden hier angezeigt.',
      testingDescription:
        'Führen Sie einen Abruftest durch, um zu prüfen, ob RAGFlow die beabsichtigten Inhalte für das LLM wiederherstellen kann.',
      similarityThreshold: 'Ähnlichkeitsschwelle',
      similarityThresholdTip:
        'RAGFlow verwendet entweder eine Kombination aus gewichteter Schlüsselwortähnlichkeit und gewichteter Vektorkosinus-Ähnlichkeit oder eine Kombination aus gewichteter Schlüsselwortähnlichkeit und gewichteter Rerank-bewertung während des Abrufs. Dieser Parameter legt den Schwellenwert für Ähnlichkeiten zwischen der Benutzeranfrage und den Chunks fest. Jeder Chunk mit einer Ähnlichkeitsbewertung unter diesem Schwellenwert wird von den Ergebnissen ausgeschlossen. Standardmäßig ist der Schwellenwert auf 0,2 festgelegt. Das bedeutet, dass nur Textblöcke mit einer hybriden Ähnlichkeitsbewertung von 20 oder höher abgerufen werden.',
      vectorSimilarityWeight: 'Schlüsselwortähnlichkeitsgewicht',
      vectorSimilarityWeightTip:
        'Dies legt das Gewicht der Schlüsselwortähnlichkeit im kombinierten Ähnlichkeitswert fest, entweder in Verbindung mit der Vektorkosinus-Ähnlichkeit oder mit der Rerank-bewertung. Die Summe der beiden Gewichte muss 1,0 ergeben.',
      keywordSimilarityWeight: 'Schlüsselwortähnlichkeitsgewicht',
      keywordSimilarityWeightTip:
        'Dies legt das Gewicht der Schlüsselwortähnlichkeit im kombinierten Ähnlichkeitswert fest, entweder in Verbindung mit der Vektorkosinus-Ähnlichkeit oder mit der Rerank-bewertung. Die Summe der beiden Gewichte muss 1,0 ergeben.',
      testText: 'Testtext',
      testTextPlaceholder: 'Geben Sie hier Ihre Frage ein!',
      testingLabel: 'Testen',
      similarity: 'Hybride Ähnlichkeit',
      termSimilarity: 'Begriffsähnlichkeit',
      vectorSimilarity: 'Vektorähnlichkeit',
      hits: 'Treffer',
      view: 'Ansehen',
      filesSelected: 'Dateien ausgewählt',
      upload: 'Hochladen',
      run: 'Analysieren',
      runningStatus0: 'AUSSTEHEND',
      runningStatus1: 'WIRD ANALYSIERT',
      runningStatus2: 'ABGEBROCHEN',
      runningStatus3: 'ERFOLGREICH',
      runningStatus4: 'FEHLGESCHLAGEN',
      pageRanges: 'Seitenbereiche',
      pageRangesTip:
        'Bereich der zu analysierenden Seiten; Seiten außerhalb dieses Bereichs werden nicht verarbeitet.',
      fromPlaceholder: 'von',
      fromMessage: 'Anfangsseitennummer fehlt',
      toPlaceholder: 'bis',
      toMessage: 'Endseitennummer fehlt (ausgeschlossen)',
      layoutRecognize: 'Dokumentenparser',
      layoutRecognizeTip:
        'Verwendet ein visuelles Modell für die PDF-Layout-Analyse, um Dokumententitel, Textblöcke, Bilder und Tabellen effektiv zu lokalisieren. Wenn die einfache Option gewählt wird, wird nur der reine Text im PDF abgerufen. Bitte beachten Sie, dass diese Option derzeit NUR für PDF-Dokumente funktioniert. Weitere Informationen finden Sie unter https://ragflow.io/docs/dev/select_pdf_parser.',
      taskPageSize: 'Aufgabenseitengröße',
      taskPageSizeMessage: 'Bitte geben Sie die Größe der Aufgabenseite ein!',
      taskPageSizeTip:
        'Während der Layouterkennung wird eine PDF-Datei in Chunks aufgeteilt und parallel verarbeitet, um die Verarbeitungsgeschwindigkeit zu erhöhen. Dieser Parameter legt die Größe jedes Chunks fest. Eine größere Chunk-Größe verringert die Wahrscheinlichkeit, dass fortlaufender Text zwischen den Seiten aufgeteilt wird.',
      addPage: 'Seite hinzufügen',
      greaterThan: 'Der aktuelle Wert muss größer als "bis" sein!',
      greaterThanPrevious:
        'Der aktuelle Wert muss größer als der vorherige "bis"-Wert sein!',
      selectFiles: 'Dateien auswählen',
      changeSpecificCategory: 'Spezifische Kategorie ändern',
      uploadTitle: 'Ziehen Sie Ihre Datei hierher, um sie hochzuladen',
      uploadDescription:
        'RAGFlow unterstützt das Hochladen von Dateien einzeln oder in Batches. Für lokal bereitgestelltes RAGFlow: Die maximale Dateigröße pro Upload beträgt 1 GB, mit einem Batch-Upload-Limit von 32 Dateien. Es gibt keine Begrenzung der Gesamtanzahl an Dateien pro Konto. Für cloud.ragflow.io: Die maximale Dateigröße pro Upload beträgt 10 MB, wobei jede Datei nicht größer als 10 MB sein darf und maximal 128 Dateien pro Konto erlaubt sind.',
      chunk: 'Chunk',
      bulk: 'Masse',
      cancel: 'Abbrechen',
      close: 'Schließen',
      rerankModel: 'Rerank-modell',
      rerankPlaceholder: 'Bitte auswählen',
      rerankTip:
        'Wenn leer gelassen, verwendet RAGFlow eine Kombination aus gewichteter Schlüsselwortähnlichkeit und gewichteter Vektorkosinus-Ähnlichkeit; wenn ein Rerank-modell ausgewählt wird, ersetzt eine gewichtete Rerank-bewertung die gewichtete Vektorkosinus-Ähnlichkeit. Bitte beachten Sie, dass die Verwendung eines Rerank-modells die Antwortzeit des Systems erheblich erhöht.',
      topK: 'Top-K',
      topKTip:
        'In Verbindung mit dem Rerank model wird mit dieser Einstellung die Anzahl der Textblöcke festgelegt, die an das angegebene reranking model gesendet werden.',
      delimiter: 'Trennzeichen für Textsegmentierung',
      delimiterTip:
        'Ein Trennzeichen oder Separator kann aus einem oder mehreren Sonderzeichen bestehen. Bei mehreren Zeichen stellen Sie sicher, dass sie in Backticks (` `) eingeschlossen sind. Wenn Sie beispielsweise Ihre Trennzeichen so konfigurieren: \\n`##`;, dann werden Ihre Texte an Zeilenumbrüchen, doppelten Rautenzeichen (##) oder Semikolons getrennt. Setzen Sie Trennzeichen nur nachdem Sie das Mechanismus der Textsegmentierung und -chunking verstanden haben.',
      enableChildrenDelimiter:
        'Untergeordnete Chunks werden für den Abruf verwendet',
      childrenDelimiter: 'Trennzeichen für Text',
      childrenDelimiterTip:
        'Ein Trennzeichen oder Separator kann aus einem oder mehreren Sonderzeichen bestehen. Bei mehreren Zeichen stellen Sie sicher, dass sie in Backticks (` `) eingeschlossen sind. Wenn Sie beispielsweise Ihre Trennzeichen so konfigurieren: \\n`##`;, dann werden Ihre Texte an Zeilenumbrüchen, doppelten Rautenzeichen (##) oder Semikolons getrennt.',
      html4excel: 'Excel zu HTML',
      html4excelTip:
        'Verwenden Sie dies zusammen mit der General-Schnittmethode. Wenn deaktiviert, werden Tabellenkalkulationsdateien (XLSX, XLS (Excel 97-2003)) zeilenweise in Schlüssel-Wert-Paare analysiert. Wenn aktiviert, werden Tabellenkalkulationsdateien in HTML-Tabellen umgewandelt. Wenn die ursprüngliche Tabelle mehr als 12 Zeilen enthält, teilt das System sie automatisch alle 12 Zeilen in mehrere HTML-Tabellen auf. Für weitere Informationen siehe https://ragflow.io/docs/dev/enable_excel2html.',
      autoKeywords: 'Auto-Schlüsselwort',
      autoKeywordsTip:
        'Extrahieren Sie automatisch N Schlüsselwörter für jeden Abschnitt, um deren Ranking in Abfragen mit diesen Schlüsselwörtern zu verbessern. Beachten Sie, dass zusätzliche Tokens vom in den "Systemmodelleinstellungen" angegebenen Chat-Modell verbraucht werden. Sie können die hinzugefügten Schlüsselwörter eines Abschnitts in der Abschnittsliste überprüfen oder aktualisieren. Für weitere Informationen siehe https://ragflow.io/docs/dev/autokeyword_autoquestion.',
      autoQuestions: 'Auto-Frage',
      autoQuestionsTip:
        'Um die Ranking-Ergebnisse zu verbessern, extrahieren Sie N Fragen für jeden Wissensdatenbank-Chunk mithilfe des im "Systemmodell-Setup" definierten Chatmodells. Beachten Sie, dass dies zusätzliche Token verbraucht. Die Ergebnisse können in der Chunk-Liste eingesehen und bearbeitet werden. Fehler bei der Fragenextraktion blockieren den Chunking-Prozess nicht; leere Ergebnisse werden dem ursprünglichen Chunk hinzugefügt. Für weitere Informationen siehe https://ragflow.io/docs/dev/autokeyword_autoquestion.',
      redo: 'Möchten Sie die vorhandenen {{chunkNum}} Chunks löschen?',
      setMetaData: 'Metadaten festlegen',
      pleaseInputJson: 'Bitte JSON eingeben',
      documentMetaTips: `<p>Die Metadaten liegen im JSON-Format vor (nicht durchsuchbar). Sie werden dem Prompt für das LLM hinzugefügt, wenn Chunks dieses Dokuments im Prompt enthalten sind.</p>
<p>Beispiele:</p>
<b>Die Metadaten sind:</b><br>
<code>
  {
      "Author": "Alex Dowson",
      "Date": "2024-11-12"
  }
</code><br>
<b>Der Prompt wird sein:</b><br>
<p>Dokument: the_name_of_document</p>
<p>Autor: Alex Dowson</p>
<p>Datum: 2024-11-12</p>
<p>Relevante Fragmente wie folgt:</p>
<ul>
<li>  Hier ist der Chunk-Inhalt....</li>
<li>  Hier ist der Chunk-Inhalt....</li>
</ul>
`,
      metaData: 'Metadaten',
      deleteDocumentConfirmContent:
        'Das Dokument ist mit dem Wissensgraphen verknüpft. Nach dem Löschen werden die zugehörigen Knoten- und Beziehungsinformationen gelöscht, aber der Graph wird nicht sofort aktualisiert. Die Aktualisierung des Graphen erfolgt während des Analyseprozesses des neuen Dokuments, das die Aufgabe zur Extraktion des Wissensgraphen enthält.',
      plainText: 'Einfach',
      reRankModelWaring: 'Das Rerank-Modell ist sehr zeitaufwendig.',
    },
    knowledgeConfiguration: {
      globalIndexModelTip:
        'Wird verwendet, um Wissensgraphen, RAPTOR, Auto-Metadaten, Auto-Schlüsselwörter und Auto-Fragen zu generieren. Die Modellleistung beeinflusst die Generierungsqualität.',
      globalIndexModel: 'Indizierungsmodell',
      settings: 'Einstellungen',
      autoMetadataTip:
        'Automatische Generierung von Metadaten. Gilt für neue Dateien während der Analyse. Vorhandene Dateien müssen neu analysiert werden, um aktualisiert zu werden (Chunks bleiben erhalten). Beachten Sie, dass zusätzliche Tokens vom in der "Konfiguration" angegebenen Indizierungsmodell verbraucht werden.',
      autoMetadata: 'Auto-Metadaten',
      mineruOptions: 'MinerU Optionen',
      mineruParseMethod: 'Analysemethode',
      mineruParseMethodTip:
        'Methode zum Parsen von PDF: auto (automatische Erkennung), txt (Textextraktion), ocr (optische Zeichenerkennung)',
      mineruFormulaEnable: 'Formelerkennung',
      mineruFormulaEnableTip:
        'Formelerkennung aktivieren. Hinweis: Dies funktioniert möglicherweise nicht korrekt bei kyrillischen Dokumenten.',
      mineruTableEnable: 'Tabellenerkennung',
      mineruTableEnableTip: 'Tabellenerkennung und -extraktion aktivieren.',
      paddleocrOptions: 'PaddleOCR-Optionen',
      paddleocrApiUrl: 'PaddleOCR API-URL',
      paddleocrApiUrlTip: 'API-Endpunkt-URL des PaddleOCR-Dienstes',
      paddleocrApiUrlPlaceholder:
        'Zum Beispiel: https://paddleocr-server.com/layout-parsing',
      paddleocrAccessToken: 'AI Studio-Zugriffstoken',
      paddleocrAccessTokenTip: 'Zugriffstoken für die PaddleOCR-API (optional)',
      paddleocrAccessTokenPlaceholder: 'Ihr AI Studio-Token (optional)',
      paddleocrAlgorithm: 'PaddleOCR-Algorithmus',
      paddleocrAlgorithmTip:
        'Algorithmus, der für die PaddleOCR-Verarbeitung verwendet wird',
      paddleocrSelectAlgorithm: 'Algorithmus auswählen',
      paddleocrModelNamePlaceholder: 'Zum Beispiel: paddleocr-umgebung-1',
      overlappedPercent: 'Chunk-Überlappung (%)',
      generationScopeTip:
        'Bestimmt, ob RAPTOR für den gesamten Datensatz oder für eine einzelne Datei generiert wird.',
      scopeDataset: 'Datensatz',
      generationScope: 'Generierungsumfang',
      scopeSingleFile: 'Einzelne Datei',
      autoParse: 'Automatisches Parsen',
      rebuildTip:
        'Lädt Dateien erneut von der verknüpften Datenquelle herunter und analysiert sie erneut.',
      baseInfo: 'Basis',
      globalIndex: 'Globaler Index',
      dataSource: 'Datenquelle',
      linkSourceSetTip:
        'Verknüpfung der Datenquelle mit diesem Datensatz verwalten',
      linkDataSource: 'Datenquelle verknüpfen',
      tocExtraction: 'Inhaltsverzeichnis verbessern',
      tocExtractionTip:
        'Für vorhandene Chunks, generieren Sie ein hierarchisches Inhaltsverzeichnis (ein Verzeichnis pro Datei). Bei Abfragen, wenn die Verzeichnisverbesserung aktiviert ist, verwendet das System ein großes Modell, um zu bestimmen, welche Verzeichniselemente für die Frage des Benutzers relevant sind, und identifiziert so die relevanten Chunks.',
      deleteGenerateModalContent: `
        <p>Das Löschen der generierten <strong class='text-text-primary'>{{type}}</strong> Ergebnisse
        entfernt alle abgeleiteten Entitäten und Beziehungen aus diesem Datensatz.
        Ihre Originaldateien bleiben intakt.<p>
        <br/>
        Möchten Sie fortfahren?
      `,
      extractRaptor: 'Raptor extrahieren',
      extractKnowledgeGraph: 'Wissensgraph extrahieren',
      filterPlaceholder: 'Bitte Filter eingeben',
      fileFilterTip: '',
      fileFilter: 'Dateifilter',
      setDefaultTip: '',
      setDefault: 'Als Standard festlegen',
      editLinkDataPipeline: 'Dateneingabe-Pipeline bearbeiten',
      linkPipelineSetTip:
        'Verknüpfung der Dateneingabe-Pipeline mit diesem Datensatz verwalten',
      default: 'Standard',
      dataPipeline: 'Wechseln oder konfigurieren Sie die Ingestion-Pipeline.',
      linkDataPipeline: 'Dateneingabe-Pipeline verknüpfen',
      enableAutoGenerate: 'Automatische Generierung aktivieren',
      teamPlaceholder: 'Bitte wählen Sie ein Team.',
      dataFlowPlaceholder: 'Bitte wählen Sie eine Pipeline.',
      buildItFromScratch: 'Von Grund auf neu erstellen',
      dataFlow: 'Pipeline',
      parseType: 'Art der Dateneingabe',
      manualSetup: 'Pipeline wählen',
      builtIn: 'eingebaute Dateneingabe',
      imageTableContextWindow: 'Kontextfenster für Bild und Tabelle',
      imageTableContextWindowTip:
        'Erfasst N Token Text ober- und unterhalb von Bild und Tabelle, um reicheren Kontext bereitzustellen.',
      titleDescription:
        'Aktualisieren Sie hier Ihre Wissensdatenbank-Konfiguration, insbesondere die Chunk-Methode.',
      name: 'Name der Wissensdatenbank',
      photo: 'Bild der Wissensdatenbank',
      photoTip: 'Sie können ein Bild bis zu 4 MB hochladen.',
      description: 'Beschreibung',
      language: 'Dokumentensprache',
      languageMessage: 'Bitte geben Sie Ihre Sprache ein!',
      languagePlaceholder: 'Bitte geben Sie Ihre Sprache ein!',
      permissions: 'Berechtigungen',
      embeddingModel: 'Embedding-Modell',
      chunkTokenNumber: 'Empfohlene Chunk-Größe',
      chunkTokenNumberMessage: 'Chunk-Token-Anzahl ist erforderlich',
      embeddingModelTip:
        'Das Standard-Embedding-Modell der Wissensdatenbank. Sobald die Wissensdatenbank Chunks enthält, führt das System beim Wechsel des Embedding-Modells eine Kompatibilitätsprüfung durch: Es zieht zufällig einige Chunks als Stichprobe, kodiert sie mit dem neuen Embedding-Modell neu und berechnet die Kosinusähnlichkeit zwischen neuen und alten Vektoren. Ein Wechsel ist nur möglich, wenn die durchschnittliche Ähnlichkeit der Stichprobe ≥ 0.9 ist. Andernfalls müssen Sie alle Chunks in der Wissensdatenbank löschen, bevor Sie das Modell ändern können.',
      permissionsTip:
        'Wenn auf "Team" gesetzt, können alle Teammitglieder die Wissensdatenbank verwalten.',
      chunkTokenNumberTip:
        'Legt den Token-Schwellenwert für einen Chunk fest. Ein Absatz mit weniger Tokens als dieser Schwellenwert wird mit dem folgenden Absatz kombiniert, bis die Token-Anzahl den Schwellenwert überschreitet, dann wird ein Chunk erstellt. Ein neuer Block wird nicht erstellt, es sei denn, ein Trennzeichen wird gefunden, auch wenn dieser Schwellenwert überschritten wird.',
      chunkMethod: 'Chunk-Methode',
      chunkMethodTip: 'Siehe Tipps auf der rechten Seite.',
      upload: 'Hochladen',
      english: 'Englisch',
      chinese: 'Chinesisch',
      portugueseBr: 'Portugiesisch (Brasilien)',
      embeddingModelPlaceholder: 'Bitte wählen Sie ein Embedding-Modell',
      chunkMethodPlaceholder: 'Bitte wählen Sie eine Chunk-Methode',
      save: 'Speichern',
      me: 'Nur ich',
      team: 'Team',
      cancel: 'Abbrechen',
      methodTitle: 'Beschreibung der Chunk-Methode',
      methodExamples: 'Beispiele',
      methodExamplesDescription:
        'Um Ihnen das Verständnis zu erleichtern, haben wir relevante Screenshots als Referenz bereitgestellt.',
      dialogueExamplesTitle: 'Dialogbeispiele',
      methodEmpty:
        'Hier wird eine visuelle Erklärung der Wissensdatenbank-Kategorien angezeigt',
      book: `<p>Unterstützte Dateiformate sind <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.</p><p>
      Für jedes Buch im PDF-Format stellen Sie bitte die <i>Seitenbereiche</i> ein, um unerwünschte Informationen zu entfernen und die Analysezeit zu reduzieren.</p>`,
      laws: `<p>Unterstützte Dateiformate sind <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.</p><p>
      Rechtliche Dokumente folgen in der Regel einem strengen Schreibformat. Wir verwenden Textmerkmale, um Teilungspunkte zu identifizieren.
      </p><p>
      Der Chunk hat eine Granularität, die mit 'ARTIKEL' übereinstimmt, wobei sichergestellt wird, dass der gesamte übergeordnete Text im Chunk enthalten ist.
      </p>`,
      manual: `<p>Nur <b>PDF</b> wird unterstützt.</p><p>
      Wir gehen davon aus, dass das Handbuch eine hierarchische Abschnittsstruktur aufweist und verwenden die Titel der untersten Abschnitte als Grundeinheit für die Aufteilung der Dokumente. Daher werden Abbildungen und Tabellen im selben Abschnitt nicht getrennt, was zu größeren Chunk-Größen führen kann.
      </p>`,
      naive: `<p>Unterstützte Dateiformate sind <b>MD, MDX, DOCX, XLSX, XLS (Excel 97-2003), PPTX, PDF, TXT, JPEG, JPG, PNG, TIF, GIF, CSV, JSON, EML, HTML</b>.</p>
      <p>Diese Methode teilt Dateien mit einer 'naiven' Methode auf: </p>
      <p>
      <li>Verwenden eines Erkennungsmodells, um die Texte in kleinere Segmente aufzuteilen.</li>
      <li>Dann werden benachbarte Segmente kombiniert, bis die Token-Anzahl den durch 'Chunk-Token-Anzahl' festgelegten Schwellenwert überschreitet, woraufhin ein Chunk erstellt wird.</li></p>`,
      paper: `<p>Nur <b>PDF</b>-Dateien werden unterstützt.</p><p>
      Papers werden nach Abschnitten wie <i>abstract, 1.1, 1.2</i> aufgeteilt. </p><p>
      Dieser Ansatz ermöglicht es dem LLM, das Paper effektiver zusammenzufassen und umfassendere, verständlichere Antworten zu liefern.
      Es erhöht jedoch auch den Kontext für KI-Gespräche und die Rechenkosten für das LLM. Daher sollten Sie während eines Gesprächs erwägen, den Wert von '<b>topN</b>' zu reduzieren.</p>`,
      presentation: `<p>Unterstützte Dateiformate sind <b>PDF</b>, <b>PPTX</b>.</p><p>
      Jede Seite in den Folien wird als Chunk behandelt, wobei ihr Vorschaubild gespeichert wird.</p><p>
      <i>Diese Chunk-Methode wird automatisch auf alle hochgeladenen PPT-Dateien angewendet, Sie müssen sie also nicht manuell angeben.</i></p>`,
      qa: `
      <p>
      Diese Chunk-Methode unterstützt die Dateiformate <b>XLSX</b> und <b>CSV/TXT</b>.
    </p>
    <li>
      Wenn eine Datei im <b>XLSX</b>-Format vorliegt, sollte sie zwei Spalten
      ohne Kopfzeilen enthalten: eine für Fragen und die andere für Antworten, wobei die
      Fragenspalte der Antwortspalte vorangeht. Mehrere Blätter sind
      akzeptabel, vorausgesetzt, die Spalten sind richtig strukturiert.
    </li>
    <li>
      Wenn eine Datei im <b>CSV/TXT</b>-Format vorliegt, muss sie UTF-8-kodiert sein und TAB als Trennzeichen verwenden, um Fragen und Antworten zu trennen.
    </li>
    <p>
      <i>
        Textzeilen, die nicht den obigen Regeln folgen, werden ignoriert, und
        jedes Frage-Antwort-Paar wird als eigenständiger Chunk betrachtet.
      </i>
    </p>
      `,
      resume: `<p>Unterstützte Dateiformate sind <b>DOCX</b>, <b>PDF</b>, <b>TXT</b>.
      </p><p>
      Lebensläufe verschiedener Formen werden analysiert und in strukturierte Daten organisiert, um die Kandidatensuche für Recruiter zu erleichtern.
      </p>
      `,
      table: `<p>Unterstützte Dateiformate sind <b>XLSX</b> und <b>CSV/TXT</b>.</p><p>
      Hier sind einige Voraussetzungen und Tipps:
      <ul>
    <li>Für CSV- oder TXT-Dateien muss das Trennzeichen zwischen den Spalten <em><b>TAB</b></em> sein.</li>
    <li>Die erste Zeile muss Spaltenüberschriften enthalten.</li>
    <li>Spaltenüberschriften müssen aussagekräftige Begriffe sein, um das Verständnis Ihres LLM zu unterstützen.
    Es ist gute Praxis, Synonyme durch einen Schrägstrich <i>'/'</i> zu trennen und Werte unter Verwendung von Klammern aufzuzählen, zum Beispiel: <i>'Gender/Sex (male, female)'</i>.<p>
    Hier sind einige Beispiele für Überschriften:<ol>
        <li>supplier/vendor<b>'TAB'</b>Color (Yellow, Blue, Brown)<b>'TAB'</b>Sex/Gender (male, female)<b>'TAB'</b>size (M, L, XL, XXL)</li>
        </ol>
        </p>
    </li>
    <li>Jede Zeile in der Tabelle wird als Chunk behandelt.</li>
    </ul>`,
      picture: `
    <p>Bilddateien werden unterstützt, Videounterstützung folgt in Kürze.</p><p>
    Diese Methode verwendet ein OCR-Modell, um Texte aus Bildern zu extrahieren.
    </p><p>
    Wenn der vom OCR-Modell extrahierte Text als unzureichend angesehen wird, wird ein bestimmtes visuelles LLM verwendet, um eine Beschreibung des Bildes zu liefern.
    </p>`,
      one: `
    <p>Unterstützte Dateiformate sind <b>DOCX, XLSX, XLS (Excel 97-2003), PDF, TXT</b>.
    </p><p>
    Diese Methode behandelt jedes Dokument in seiner Gesamtheit als einen Chunk.
    </p><p>
    Anwendbar, wenn Sie das LLM das gesamte Dokument zusammenfassen lassen möchten, vorausgesetzt, es kann mit dieser Kontextlänge umgehen.
    </p>`,
      knowledgeGraph: `<p>Unterstützte Dateiformate sind <b>DOCX, EXCEL, PPT, IMAGE, PDF, TXT, MD, JSON, EML</b>

<p>Dieser Ansatz teilt Dateien mit der 'naiven'/'Allgemeinen' Methode auf. Er teilt ein Dokument in Segmente und kombiniert dann benachbarte Segmente, bis die Token-Anzahl den durch 'Chunk-Token-Anzahl' festgelegten Schwellenwert überschreitet, woraufhin ein Chunk erstellt wird.</p>
<p>Die Chunks werden dann dem LLM zugeführt, um Entitäten und Beziehungen für einen Wissensgraphen und eine Mind Map zu extrahieren.</p>
<p>Stellen Sie sicher, dass Sie die <b>Entitätstypen</b> festlegen.</p>`,
      tag: `<p>Eine Wissensdatenbank, die die 'Tag'-Chunk-Methode verwendet, fungiert als Tag-Set. Andere Wissensdatenbanken können es verwenden, um ihre eigenen Chunks zu taggen, und Abfragen an diese Wissensdatenbanken werden ebenfalls mit diesem Tag-Set getaggt.</p>
<p>Ein Tag-Set wird <b>NICHT</b> direkt in einen Retrieval-Augmented Generation (RAG)-Prozess einbezogen.</p>
<p>Jeder Chunk in dieser Wissensdatenbank ist ein unabhängiges Beschreibungs-Tag-Paar.</p>
<p>Zu den unterstützten Dateiformaten gehören <b>XLSX</b> und <b>CSV/TXT</b>:</p>
<p>Wenn eine Datei im <b>XLSX</b>-Format vorliegt, sollte sie zwei Spalten ohne Überschriften enthalten: eine für Tag-Beschreibungen und die andere für Tag-Namen, wobei die Spalte "Beschreibung" vor der Spalte "Tag" steht. Mehrere Blätter sind zulässig, sofern die Spalten ordnungsgemäß strukturiert sind.</p>
<p>Wenn eine Datei im <b>CSV/TXT</b>-Format vorliegt, muss sie UTF-8-kodiert sein, wobei TAB als Trennzeichen zum Trennen von Beschreibungen und Tags verwendet wird.</p>
<p>In einer Tag-Spalte wird das <b>Komma</b> verwendet, um Tags zu trennen.</p>
<i>Textzeilen, die nicht den obigen Regeln entsprechen, werden ignoriert.</i>
`,
      useRaptor: 'RAPTOR zur Verbesserung des Abrufs verwenden',
      useRaptorTip:
        'RAPTOR für Multi-Hop-Frage-Antwort-Aufgaben aktivieren. Details unter https://ragflow.io/docs/dev/enable_raptor.',
      prompt: 'Prompt',
      promptTip:
        'Verwenden Sie den Systemprompt, um die Aufgabe für das LLM zu beschreiben, festzulegen, wie es antworten soll, und andere verschiedene Anforderungen zu skizzieren. Der Systemprompt wird oft in Verbindung mit Schlüsseln (Variablen) verwendet, die als verschiedene Dateninputs für das LLM dienen. Verwenden Sie einen Schrägstrich `/` oder die (x)-Schaltfläche, um die zu verwendenden Schlüssel anzuzeigen.',
      promptMessage: 'Prompt ist erforderlich',
      promptText: `Bitte fassen Sie die folgenden Absätze zusammen. Seien Sie vorsichtig mit den Zahlen, erfinden Sie keine Dinge. Absätze wie folgt:
      {cluster_content}
Das oben Genannte ist der Inhalt, den Sie zusammenfassen müssen.`,
      maxToken: 'Maximale Token',
      maxTokenTip:
        'Die maximale Anzahl an Token pro generiertem Zusammenfassungs-Chunk.',
      maxTokenMessage: 'Maximale Token sind erforderlich',
      threshold: 'Schwellenwert',
      thresholdTip:
        'In RAPTOR werden Chunks anhand ihrer semantischen Ähnlichkeit gruppiert. Der Schwellenwert-Parameter legt die minimale Ähnlichkeit fest, die erforderlich ist, damit Chunks zusammengefasst werden. Ein höherer Schwellenwert bedeutet weniger Chunks pro Cluster, während ein niedrigerer Wert mehr Chunks pro Cluster zulässt.',
      thresholdMessage: 'Schwellenwert ist erforderlich',
      maxCluster: 'Maximale Cluster',
      maxClusterTip: 'Die maximale Anzahl der zu erstellenden Cluster.',
      maxClusterMessage: 'Maximale Cluster sind erforderlich',
      randomSeed: 'Zufallszahl',
      randomSeedMessage: 'Zufallszahl ist erforderlich',
      entityTypes: 'Entitätstypen',
      vietnamese: 'Vietnamesisch',
      pageRank: 'PageRank',
      pageRankTip:
        'Sie können während des Abrufs bestimmten Wissensdatenbanken eine höhere PageRank-Bewertung zuweisen. Die entsprechende Bewertung wird zu den hybriden Ähnlichkeitswerten der abgerufenen Chunks aus diesen Wissensdatenbanken addiert, wodurch deren Ranking erhöht wird. Weitere Informationen finden Sie unter https://ragflow.io/docs/dev/set_page_rank.',
      tagName: 'Tag',
      frequency: 'Häufigkeit',
      searchTags: 'Tags durchsuchen',
      tagCloud: 'Wolke',
      tagTable: 'Tabelle',
      tagSet: 'Tag-Sets',
      tagSetTip: `
     <p> Wählen Sie eine oder mehrere Tag-Wissensdatenbanken aus, um Chunks in Ihrer Wissensdatenbank automatisch zu taggen. </p>
<p>Die Benutzeranfrage wird ebenfalls automatisch getaggt.</p>
Diese Auto-Tag-Funktion verbessert den Abruf, indem sie eine weitere Schicht domänenspezifischen Wissens zum vorhandenen Datensatz hinzufügt.
<p>Unterschied zwischen Auto-Tag und Auto-Schlüsselwort:</p>
<ul>
  <li>Eine Tag-Wissensdatenbank ist ein benutzerdefiniertes geschlossenes Set, während vom LLM extrahierte Schlüsselwörter als offenes Set betrachtet werden können.</li>
  <li>Sie müssen Tag-Sets in bestimmten Formaten hochladen, bevor Sie die Auto-Tag-Funktion ausführen.</li>
  <li>Die Auto-Schlüsselwort-Funktion ist vom LLM abhängig und verbraucht eine erhebliche Anzahl an Tokens.</li>
</ul>
<p>Siehe https://ragflow.io/docs/dev/use_tag_sets für Details.</p>
      `,
      topnTags: 'Top-N Tags',
      tags: 'Tags',
      addTag: 'Tag hinzufügen',
      useGraphRag: 'Wissensgraph extrahieren',
      useGraphRagTip:
        'Erstellen Sie einen Wissensgraph über Dateiabschnitte der aktuellen Wissensbasis, um die Beantwortung von Fragen mit mehreren Schritten und verschachtelter Logik zu verbessern. Weitere Informationen finden Sie unter https://ragflow.io/docs/dev/construct_knowledge_graph.',
      graphRagMethod: 'Methode',
      graphRagMethodTip: `
      Light: (Standard) Verwendet von github.com/HKUDS/LightRAG bereitgestellte Prompts, um Entitäten und Beziehungen zu extrahieren. Diese Option verbraucht weniger Tokens, weniger Speicher und weniger Rechenressourcen.</br>
      General: Verwendet von github.com/microsoft/graphrag bereitgestellte Prompts, um Entitäten und Beziehungen zu extrahieren`,
      resolution: 'Entitätsauflösung',
      resolutionTip: `Ein Entitäts-Deduplizierungsschalter. Wenn aktiviert, wird das LLM ähnliche Entitäten kombinieren - z.B. '2025' und 'das Jahr 2025' oder 'IT' und 'Informationstechnologie' - um einen genaueren Graphen zu konstruieren`,
      community: 'Generierung von Gemeinschaftsberichten',
      communityTip:
        'In einem Wissensgraphen ist eine Gemeinschaft ein Cluster von Entitäten, die durch Beziehungen verbunden sind. Sie können das LLM eine Zusammenfassung für jede Gemeinschaft erstellen lassen, bekannt als Gemeinschaftsbericht. Weitere Informationen finden Sie hier: https://www.microsoft.com/en-us/research/blog/graphrag-improving-global-search-via-dynamic-community-selection/',
      theDocumentBeingParsedCannotBeDeleted:
        'Das Dokument, das gerade analysiert wird, kann nicht gelöscht werden',
      lastWeek: 'von letzter Woche',
    },
    chunk: {
      type: 'Typ',
      docType: {
        image: 'Bild',
        table: 'Tabelle',
        text: 'Text',
      },
      chunk: 'Chunk',
      bulk: 'Masse',
      selectAll: 'Alle auswählen',
      enabledSelected: 'Ausgewählte aktivieren',
      disabledSelected: 'Ausgewählte deaktivieren',
      deleteSelected: 'Ausgewählte löschen',
      search: 'Suchen',
      all: 'Alle',
      enabled: 'Aktiviert',
      disabled: 'Deaktiviert',
      keyword: 'Schlüsselwort',
      image: 'Bild',
      imageUploaderTitle:
        'Laden Sie ein neues Bild hoch, um diesen Bild-Chunk zu aktualisieren',
      function: 'Funktion',
      chunkMessage: 'Bitte Wert eingeben!',
      full: 'Volltext',
      ellipse: 'Ellipse',
      graph: 'Wissensgraph',
      mind: 'Mind Map',
      question: 'Frage',
      questionTip:
        'Wenn vorgegebene Fragen vorhanden sind, basiert das Embedding des Chunks auf diesen.',
      chunkResult: 'Chunk-Ergebnis',
      chunkResultTip:
        'Sehen Sie sich die gechunkten Segmente an, die für Embedding und Abruf verwendet werden.',
      enable: 'Aktivieren',
      disable: 'Deaktivieren',
      delete: 'Löschen',
    },
    chat: {
      messagePlaceholder: 'Geben Sie hier Ihre Nachricht ein...',
      exit: 'Verlassen',
      multipleModels: 'Mehrere Modelle',
      applyModelConfigs: 'Modellkonfigurationen anwenden',
      conversations: 'Unterhaltungen',
      chatApps: 'Chat-Apps',
      newConversation: 'Neue Unterhaltung',
      createAssistant: 'Assistenten erstellen',
      assistantSetting: 'Assistenteneinstellung',
      promptEngine: 'Prompt-Engine',
      modelSetting: 'Modelleinstellung',
      chat: 'Chat',
      newChat: 'Neuer Chat',
      send: 'Senden',
      sendPlaceholder: 'Nachricht an den Assistenten...',
      chatConfiguration: 'Chat-Konfiguration',
      chatConfigurationDescription:
        'Richten Sie einen Chat-Assistenten für die ausgewählten Datensätze (Wissensbasen) hier ein! 💕',
      assistantName: 'Assistentenname',
      assistantNameMessage: 'Assistentenname ist erforderlich',
      namePlaceholder: 'z.B. Lebenslauf-Jarvis',
      assistantAvatar: 'Assistentenbild',
      language: 'Sprache',
      emptyResponse: 'Leere Antwort',
      emptyResponseTip:
        'Legen Sie dies als Antwort fest, wenn keine Ergebnisse aus den Wissensdatenbanken für Ihre Anfrage abgerufen werden, oder lassen Sie dieses Feld leer, damit das LLM improvisieren kann, wenn nichts gefunden wird.',
      emptyResponseMessage:
        'Eine leere Antwort wird ausgelöst, wenn nichts Relevantes aus den Wissensdatenbanken abgerufen wird. Sie müssen das Feld "Leere Antwort" löschen, wenn keine Wissensdatenbank ausgewählt ist.',
      setAnOpener: 'Begrüßungstext',
      setAnOpenerInitial:
        'Hallo! Ich bin Ihr Assistent, was kann ich für Sie tun?',
      setAnOpenerTip: 'Legen Sie einen Begrüßungstext für Benutzer fest.',
      knowledgeBases: 'Wissensdatenbanken',
      knowledgeBasesMessage: 'Bitte auswählen',
      knowledgeBasesTip:
        'Wählen Sie die Wissensdatenbanken aus, die mit diesem Chat-Assistenten verknüpft werden sollen. Eine leere Wissensdatenbank wird nicht in der Dropdown-Liste angezeigt.',
      system: 'System',
      systemInitialValue:
        'Sie sind ein intelligenter Assistent. Bitte fassen Sie den Inhalt der Wissensdatenbank zusammen, um die Frage zu beantworten. Bitte listen Sie die Daten in der Wissensdatenbank auf und antworten Sie detailliert. Wenn alle Inhalte der Wissensdatenbank für die Frage irrelevant sind, muss Ihre Antwort den Satz "Die gesuchte Antwort wurde in der Wissensdatenbank nicht gefunden!" enthalten. Antworten müssen den Chat-Verlauf berücksichtigen.\nHier ist die Wissensdatenbank:\n{knowledge}\nDas oben Genannte ist die Wissensdatenbank.',
      systemMessage: 'Bitte eingeben!',
      systemTip:
        'Ihre Prompts oder Anweisungen für das LLM, einschließlich, aber nicht beschränkt auf seine Rolle, die gewünschte Länge, den Ton und die Sprache seiner Antworten. Wenn Ihr Modell native Unterstützung für das Schlussfolgern hat, können Sie //no_thinking zum Prompt hinzufügen, um das Schlussfolgern zu stoppen.',
      topN: 'Top N',
      topNTip:
        'Nicht alle Chunks mit einem Ähnlichkeitswert über dem "Ähnlichkeitsschwellenwert" werden an das LLM gesendet. Dies wählt die "Top N" Chunks aus den abgerufenen aus.',
      variable: 'Variable',
      variableTip:
        'In Kombination mit den APIs zur Verwaltung von Chat-Assistenten von RAGFlow können Variablen dazu beitragen, flexiblere System-Prompt-Strategien zu entwickeln. Die definierten Variablen werden von „System-Prompt“ als Teil der Prompts für das LLM verwendet. {knowledge} ist eine spezielle reservierte Variable, die Teile darstellt, die aus den angegebenen Wissensbasen abgerufen werden, und alle Variablen sollten in geschweiften Klammern {} im „System-Prompt“ eingeschlossen werden. Weitere Informationen finden Sie unter https://ragflow.io/docs/dev/set_chat_variables.',
      add: 'Hinzufügen',
      key: 'Schlüssel',
      optional: 'Optional',
      operation: 'Operation',
      model: 'Modell',
      modelTip: 'Großes Sprachmodell für Chat',
      modelMessage: 'Bitte auswählen!',
      modelEnabledTools: 'Aktivierte Tools',
      modelEnabledToolsTip:
        'Bitte wählen Sie ein oder mehrere Tools aus, die das Chat-Modell verwenden soll. Dies hat keine Auswirkung auf Modelle, die keinen Tool-Aufruf unterstützen.',
      freedom: 'Freiheit',
      improvise: 'Improvisieren',
      precise: 'Präzise',
      balance: 'Ausgewogen',
      custom: 'Benutzerdefiniert',
      freedomTip:
        'Eine Abkürzung für die Einstellungen "Temperatur", "Top P", "Präsenzstrafe" und "Häufigkeitsstrafe", die den Freiheitsgrad des Modells angibt. Dieser Parameter hat drei Optionen: Wählen Sie "Improvisieren", um kreativere Antworten zu erzeugen; wählen Sie "Präzise" (Standard), um konservativere Antworten zu erzeugen; "Ausgewogen" ist ein Mittelweg zwischen "Improvisieren" und "Präzise".',
      temperature: 'Temperatur',
      temperatureMessage: 'Temperatur ist erforderlich',
      temperatureTip:
        'Dieser Parameter steuert die Zufälligkeit der Vorhersagen des Modells. Eine niedrigere Temperatur führt zu konservativeren Antworten, während eine höhere Temperatur kreativere und vielfältigere Antworten liefert.',
      topP: 'Top P',
      topPMessage: 'Top P ist erforderlich',
      topPTip:
        'Auch bekannt als "Nucleus-Sampling", setzt dieser Parameter einen Schwellenwert für die Auswahl einer kleineren Menge der wahrscheinlichsten Wörter, aus denen Stichproben genommen werden sollen, und schneidet die weniger wahrscheinlichen ab.',
      presencePenalty: 'Präsenzstrafe',
      presencePenaltyMessage: 'Präsenzstrafe ist erforderlich',
      presencePenaltyTip:
        'Dies entmutigt das Modell, dieselben Informationen zu wiederholen, indem es Wörter bestraft, die bereits im Gespräch vorgekommen sind.',
      frequencyPenalty: 'Häufigkeitsstrafe',
      frequencyPenaltyMessage: 'Häufigkeitsstrafe ist erforderlich',
      frequencyPenaltyTip:
        'Ähnlich wie die Präsenzstrafe reduziert dies die Tendenz des Modells, dieselben Wörter häufig zu wiederholen.',
      maxTokens: 'Maximale Tokens',
      maxTokensMessage: 'Maximale Tokens sind erforderlich',
      maxTokensTip: `Die maximale Kontextgröße des Modell; ein ungültiger oder falscher Wert führt zu einem Fehler. Standardmäßig 512.`,
      maxTokensInvalidMessage:
        'Bitte geben Sie eine gültige Zahl für Maximale Tokens ein.',
      maxTokensMinMessage: 'Maximale Tokens können nicht weniger als 0 sein.',
      quote: 'Zitat anzeigen',
      quoteTip: 'Ob der Originaltext als Referenz angezeigt werden soll.',
      selfRag: 'Self-RAG',
      selfRagTip:
        'Bitte beziehen Sie sich auf: https://huggingface.co/papers/2310.11511',
      overview: 'Chat-ID',
      pv: 'Anzahl der Nachrichten',
      uv: 'Anzahl aktiver Benutzer',
      speed: 'Token-Ausgabegeschwindigkeit',
      tokens: 'Verbrauchte Token-Anzahl',
      round: 'Anzahl der Sitzungsinteraktionen',
      thumbUp: 'Kundenzufriedenheit',
      preview: 'Vorschau',
      embedded: 'Eingebettet',
      serviceApiEndpoint: 'Service-API-Endpunkt',
      apiKey: 'API-SCHLÜSSEL',
      apiReference: 'API-Dokumente',
      dateRange: 'Datumsbereich:',
      backendServiceApi: 'API-Server',
      createNewKey: 'Neuen Schlüssel erstellen',
      created: 'Erstellt',
      action: 'Aktion',
      embedModalTitle: 'In Webseite einbetten',
      comingSoon: 'Demnächst verfügbar',
      fullScreenTitle: 'Vollständige Einbettung',
      fullScreenDescription:
        'Betten Sie den folgenden iframe an der gewünschten Stelle in Ihre Website ein',
      partialTitle: 'Teilweise Einbettung',
      extensionTitle: 'Chrome-Erweiterung',
      tokenError: 'Bitte erstellen Sie zuerst einen API-Schlüssel.',
      betaError:
        'Bitte erwerben Sie zuerst einen RAGFlow-API-Schlüssel auf der Systemeinstellungsseite.',
      searching: 'Suche...',
      parsing: 'Analysiere',
      uploading: 'Hochladen',
      uploadFailed: 'Hochladen fehlgeschlagen',
      regenerate: 'Neu generieren',
      read: 'Inhalt lesen',
      tts: 'Text zu Sprache',
      ttsTip:
        'Stellen Sie sicher, dass Sie ein TTS-Modell auf der Einstellungsseite auswählen, bevor Sie diesen Schalter aktivieren, um Text als Audio abzuspielen.',
      relatedQuestion: 'Verwandte Frage',
      answerTitle: 'A',
      multiTurn: 'Mehrfach-Runden-Optimierung',
      multiTurnTip:
        'Dies optimiert Benutzeranfragen unter Verwendung des Kontexts in einer mehrrundigen Unterhaltung. Wenn aktiviert, werden zusätzliche LLM-Tokens verbraucht.',
      howUseId: 'Wie verwendet man die Chat-ID?',
      description: 'Beschreibung des Assistenten',
      descriptionPlaceholder: 'z.B. Ein Chat-Assistent für Lebensläufe.',
      useKnowledgeGraph: 'Wissensgraph verwenden',
      useKnowledgeGraphTip:
        'Ob ein Wissensgraph im angegebenen Wissensspeicher während der Wiederherstellung für die Beantwortung von Fragen mit mehreren Schritten verwendet werden soll. Wenn aktiviert, beinhaltet dies iterative Suchen über Entitäten-, Beziehungs- und Gemeinschaftsberichtssegmente, was die Wiederherstellungszeit erheblich verlängert.',
      keyword: 'Schlüsselwortanalyse',
      keywordTip:
        'LLM anwenden, um die Fragen des Benutzers zu analysieren und Schlüsselwörter zu extrahieren, die während der Relevanzberechnung hervorgehoben werden. Funktioniert gut bei langen Anfragen, erhöht jedoch die Antwortzeit.',
      languageTip:
        'Ermöglicht die Umformulierung von Sätzen in der angegebenen Sprache oder verwendet standardmäßig die letzte Frage, wenn keine ausgewählt ist.',
      avatarHidden: 'Avatar ausblenden',
      locale: 'Gebietsschema',
      selectLanguage: 'Sprache auswählen',
      reasoning: 'Schlussfolgerung',
      reasoningTip:
        'Ob beim Frage-Antwort-Prozess ein logisches Arbeitsverfahren aktiviert werden soll, wie es bei Modellen wie Deepseek-R1 oder OpenAI o1 der Fall ist. Wenn aktiviert, ermöglicht diese Funktion dem Modell, auf externes Wissen zuzugreifen und komplexe Fragen schrittweise mithilfe von Techniken wie der „Chain-of-Thought“-Argumentation zu lösen. Durch die Zerlegung von Problemen in überschaubare Schritte verbessert dieser Ansatz die Fähigkeit des Modells, präzise Antworten zu liefern, was die Leistung bei Aufgaben, die logisches Denken und mehrschrittige Überlegungen erfordern, steigert.',
      tavilyApiKeyTip:
        'Wenn hier ein API-Schlüssel korrekt eingestellt ist, werden Tavily-basierte Websuchen verwendet, um den Abruf aus der Wissensdatenbank zu ergänzen.',
      tavilyApiKeyMessage: 'Bitte geben Sie Ihren Tavily-API-Schlüssel ein',
      tavilyApiKeyHelp: 'Wie bekomme ich ihn?',
      crossLanguage: 'Sprachübergreifende Suche',
      crossLanguageTip:
        'Wählen Sie eine oder mehrere Sprachen für die sprachübergreifende Suche aus. Wenn keine Sprache ausgewählt ist, sucht das System mit der ursprünglichen Abfrage.',
      createChat: 'Chat erstellen',
      metadata: 'Metadaten',
      metadataTip:
        'Metadatenfilterung ist der Prozess der Verwendung von Metadatenattributen (wie Tags, Kategorien oder Zugriffsberechtigungen), um den Abruf relevanter Informationen innerhalb eines Systems zu verfeinern und zu steuern.',
      conditions: 'Bedingungen',
      metadataKeys: 'Filterbare Elemente',
      addCondition: 'Bedingung hinzufügen',
      meta: {
        disabled: 'Deaktiviert',
        auto: 'Automatisch',
        manual: 'Manuell',
        semi_auto: 'Halbautomatisch',
      },
      cancel: 'Abbrechen',
      chatSetting: 'Chat-Einstellung',
      tocEnhance: 'Inhaltsverzeichnis verbessern',
      tocEnhanceTip:
        'Während der Analyse des Dokuments wurden Inhaltsverzeichnisinformationen generiert (siehe Option "Inhaltsverzeichnis-Extraktion aktivieren" in der allgemeinen Methode). Dies ermöglicht es dem großen Modell, Inhaltsverzeichniselemente zurückzugeben, die für die Abfrage des Benutzers relevant sind, und diese Elemente zu verwenden, um verwandte Chunks abzurufen und diese Chunks während des Sortiervorgangs zu gewichten. Dieser Ansatz leitet sich von der Nachahmung der Verhaltenslogik ab, wie Menschen in Büchern nach Wissen suchen.',
      batchDeleteSessions: 'Stapel löschen',
      deleteSelectedConfirm: 'Die ausgewählten {count} Sitzung(en) löschen?',
    },
    setting: {
      deleteModel: 'Modell löschen',
      bedrockCredentialsHint:
        'Tipp: Lassen Sie Access Key / Secret Key leer, um AWS IAM-Authentifizierung zu verwenden.',
      awsAuthModeAccessKeySecret: 'Access Key',
      awsAuthModeIamRole: 'IAM Role',
      awsAuthModeAssumeRole: 'Assume Role',
      awsAccessKeyId: 'AWS Access Key ID',
      awsSecretAccessKey: 'AWS Secret Access Key',
      awsRoleArn: 'AWS Role ARN',
      awsRoleArnMessage: 'Bitte geben Sie die AWS Role ARN ein',
      awsAssumeRoleTip:
        'Wenn Sie diesen Modus wählen, übernimmt die Amazon EC2-Instanz ihre bestehende Rolle, um auf AWS-Dienste zuzugreifen. Es sind keine zusätzlichen Anmeldeinformationen erforderlich.',
      modelEmptyTip:
        'Keine Modelle verfügbar. <br>Bitte fügen Sie Modelle über das Panel auf der rechten Seite hinzu.',
      sourceEmptyTip:
        'Noch keine Datenquellen hinzugefügt. Wählen Sie unten eine aus, um eine Verbindung herzustellen.',
      seconds: 'Sekunden',
      minutes: 'Minuten',
      edit: 'Bearbeiten',
      cropTip:
        'Ziehen Sie den Auswahlbereich, um die Zuschneideposition des Bildes zu wählen, und scrollen Sie zum Vergrößern/Verkleinern',
      cropImage: 'Bild zuschneiden',
      selectModelPlaceholder: 'Modell auswählen',
      configureModelTitle: 'Modell konfigurieren',
      confluenceIsCloudTip:
        'Aktivieren Sie dies, wenn es sich um eine Confluence Cloud-Instanz handelt, deaktivieren Sie es für Confluence Server/Data Center',
      confluenceWikiBaseUrlTip:
        'Die Basis-URL Ihrer Confluence-Instanz (z.B. https://your-domain.atlassian.net/wiki)',
      confluenceSpaceKeyTip:
        'Optional: Geben Sie einen Space-Key an, um die Synchronisierung auf einen bestimmten Bereich zu beschränken. Lassen Sie das Feld leer, um alle zugänglichen Bereiche zu synchronisieren. Trennen Sie mehrere Bereiche durch Kommas (z.B. DEV,DOCS,HR)',
      s3PrefixTip: `Geben Sie den Ordnerpfad innerhalb Ihres S3-Buckets an, aus dem Dateien abgerufen werden sollen.
Beispiel: general/v2/`,
      S3CompatibleEndpointUrlTip: `Erforderlich für S3-kompatible Storage Box. Geben Sie die S3-kompatible Endpunkt-URL an.
Beispiel: https://fsn1.your-objectstorage.com`,
      S3CompatibleAddressingStyleTip: `Erforderlich für S3-kompatible Storage Box. Geben Sie den S3-kompatiblen Adressierungsstil an.
Beispiel: Virtual Hosted Style`,
      addDataSourceModalTitle: 'Erstellen Sie Ihren {{name}} Connector',
      deleteSourceModalTitle: 'Datenquelle löschen',
      deleteSourceModalContent: `
      <p>Sind Sie sicher, dass Sie diese Datenquellenverknüpfung löschen möchten?</p>`,
      deleteSourceModalConfirmText: 'Bestätigen',
      errorMsg: 'Fehlermeldung',
      newDocs: 'Neue Dokumente',
      timeStarted: 'Startzeit',
      log: 'Log',
      confluenceDescription:
        'Integrieren Sie Ihren Confluence-Arbeitsbereich, um Dokumentationen zu durchsuchen.',
      s3Description:
        'Verbinden Sie sich mit Ihrem AWS S3-Bucket, um gespeicherte Dateien zu importieren und zu synchronisieren.',
      google_cloud_storageDescription:
        'Verbinden Sie Ihren Google Cloud Storage-Bucket, um Dateien zu importieren und zu synchronisieren.',
      r2Description:
        'Verbinden Sie Ihren Cloudflare R2-Bucket, um Dateien zu importieren und zu synchronisieren.',
      oci_storageDescription:
        'Verbinden Sie Ihren Oracle Cloud Object Storage-Bucket, um Dateien zu importieren und zu synchronisieren.',
      discordDescription:
        'Verknüpfen Sie Ihren Discord-Server, um auf Chat-Daten zuzugreifen und diese zu analysieren.',
      notionDescription:
        'Synchronisieren Sie Seiten und Datenbanken von Notion für den Wissensabruf.',
      google_driveDescription:
        'Verbinden Sie Ihr Google Drive über OAuth und synchronisieren Sie bestimmte Ordner oder Laufwerke.',
      gmailDescription:
        'Verbinden Sie Ihr Gmail über OAuth, um E-Mails zu synchronisieren.',
      webdavDescription:
        'Verbinden Sie sich mit WebDAV-Servern, um Dateien zu synchronisieren.',
      gitlabDescription:
        'Verbinden Sie GitLab, um Repositories, Issues, Merge Requests und zugehörige Dokumentation zu synchronisieren.',
      webdavRemotePathTip:
        'Optional: Geben Sie einen Ordnerpfad auf dem WebDAV-Server an (z.B. /Dokumente). Lassen Sie das Feld leer, um vom Stammverzeichnis aus zu synchronisieren.',
      google_driveTokenTip:
        'Laden Sie das OAuth-Token-JSON hoch, das vom OAuth-Helper oder der Google Cloud Console generiert wurde. Sie können auch ein client_secret JSON von einer "installierten" oder "Web"-Anwendung hochladen. Wenn dies Ihre erste Synchronisierung ist, öffnet sich ein Browserfenster, um die OAuth-Zustimmung abzuschließen. Wenn das JSON bereits ein Refresh-Token enthält, wird es automatisch wiederverwendet.',
      google_drivePrimaryAdminTip:
        'E-Mail-Adresse, die Zugriff auf den zu synchronisierenden Drive-Inhalt hat.',
      google_driveMyDriveEmailsTip:
        'Kommagetrennte E-Mails, deren "My Drive"-Inhalte indiziert werden sollen (einschließlich des primären Admins).',
      google_driveSharedFoldersTip:
        'Kommagetrennte Google Drive-Ordnerlinks zum Crawlen.',
      gmailPrimaryAdminTip:
        'Primäre Admin-E-Mail mit Gmail / Workspace-Zugriff, die verwendet wird, um Domänenbenutzer aufzulisten und als Standard-Synchronisierungskonto dient.',
      gmailTokenTip:
        'Laden Sie das OAuth-JSON hoch, das von der Google Console generiert wurde. Wenn es nur Client-Anmeldeinformationen enthält, führen Sie die browserbasierte Überprüfung einmal durch, um langlebige Refresh-Token zu erstellen.',
      dropboxDescription:
        'Verbinden Sie Ihre Dropbox, um Dateien und Ordner von einem ausgewählten Konto zu synchronisieren.',
      bitbucketDescription:
        'Bitbucket verbinden, um PR-Inhalte zu synchronisieren.',
      zendeskDescription:
        'Verbinden Sie Ihr Zendesk, um Tickets, Artikel und andere Inhalte zu synchronisieren.',
      bitbucketTopWorkspaceTip:
        'Der zu indizierende Bitbucket-Workspace (z. B. "atlassian" aus https://bitbucket.org/atlassian/workspace )',
      bitbucketWorkspaceTip:
        'Dieser Connector indiziert alle Repositories im Workspace.',
      bitbucketProjectsTip: 'Kommagetrennte Projekt-Keys, z. B.: PROJ1,PROJ2',
      bitbucketRepositorySlugsTip:
        'Kommagetrennte Repository-Slugs, z. B.: repo-one,repo-two',
      connectorNameTip:
        'Geben Sie einen aussagekräftigen Namen für den Connector an',
      boxDescription:
        'Verbinden Sie Ihr Box-Laufwerk, um Dateien und Ordner zu synchronisieren.',
      githubDescription:
        'Verbinden Sie GitHub, um Pull Requests und Issues zur Recherche zu synchronisieren.',
      airtableDescription:
        'Verbinden Sie sich mit Airtable und synchronisieren Sie Dateien aus einer bestimmten Tabelle in einem vorgesehenen Arbeitsbereich.',
      dingtalkAITableDescription:
        'Verbinden Sie sich mit Dingtalk AI Table und synchronisieren Sie Datensätze aus einer bestimmten Tabelle.',
      asanaDescription:
        'Verbinden Sie sich mit Asana und synchronisieren Sie Dateien aus einem bestimmten Arbeitsbereich.',
      imapDescription:
        'Verbinden Sie sich mit Ihrem IMAP-Postfach, um E-Mails für den Wissensabruf zu synchronisieren.',
      dropboxAccessTokenTip:
        'Generieren Sie ein langlebiges Zugriffstoken in der Dropbox App Console mit den Bereichen files.metadata.read, files.content.read und sharing.read.',
      moodleDescription:
        'Verbinden Sie sich mit Ihrem Moodle LMS, um Kursinhalte, Foren und Ressourcen zu synchronisieren.',
      moodleUrlTip:
        'Die Basis-URL Ihrer Moodle-Instanz (z.B. https://moodle.university.edu). Fügen Sie nicht /webservice oder /login hinzu.',
      moodleTokenTip:
        'Generieren Sie ein Web-Service-Token in Moodle: Gehen Sie zu Website-Administration → Server → Web-Services → Token verwalten. Der Benutzer muss in den Kursen eingeschrieben sein, die Sie synchronisieren möchten.',
      seafileDescription:
        'Verbinden Sie sich mit Ihrem SeaFile-Server, um Dateien und Dokumente aus Ihren Bibliotheken zu synchronisieren.',
      seafileUrlTip:
        'Die vollstaendige URL Ihres SeaFile-Servers inklusive Protokoll. Beispiel: https://seafile.example.com - Kein abschliessender Schraegstrich und kein Pfad nach der Domain.',
      seafileAccountScopeTip:
        'Synchronisiert alle Bibliotheken, die für den unten angegebenen Konto-API-Token sichtbar sind.',
      seafileTokenPanelHeading:
        'Wählen Sie eine der folgenden Authentifizierungsmethoden:',
      seafileTokenPanelAccountBullet:
        '- gewährt Zugriff auf alle Ihre Bibliotheken.',
      seafileTokenPanelLibraryBullet:
        '- auf eine einzelne Bibliothek beschränkt (sicherer).',
      seafileValidationAccountTokenRequired:
        'Konto-API-Token ist erforderlich für den Umfang „Gesamtes Konto"',
      seafileValidationTokenRequired:
        'Geben Sie entweder einen Konto-API-Token oder einen Bibliotheks-Token an',
      seafileValidationLibraryIdRequired: 'Bibliotheks-ID ist erforderlich',
      seafileValidationDirectoryPathRequired:
        'Verzeichnispfad ist erforderlich',
      seafileSyncScopeTip:
        'Legt fest, was synchronisiert wird: ' +
        '(1) Gesamtes Konto - Synchronisiert alle Bibliotheken, auf die Ihr Token Zugriff hat. Erfordert einen Konto-API-Token. ' +
        '(2) Einzelne Bibliothek - Synchronisiert alle Dateien innerhalb einer bestimmten Bibliothek. Erfordert die Bibliotheks-ID und entweder einen Konto-API-Token oder einen Bibliotheks-API-Token. ' +
        '(3) Bestimmtes Verzeichnis - Synchronisiert nur Dateien in einem bestimmten Ordner innerhalb einer Bibliothek. Erfordert die Bibliotheks-ID, den Ordnerpfad innerhalb dieser Bibliothek und entweder einen Konto-API-Token oder einen Bibliotheks-API-Token.',
      seafileTokenTip:
        'Ihr kontoweiter SeaFile-API-Token. ' +
        'Gewährt Zugriff auf alle fuer Ihr Konto sichtbaren Bibliotheken. ' +
        'Erforderlich bei Synchronisierungsumfang "Gesamtes Konto". ' +
        'Für "Einzelne Bibliothek" oder "Bestimmtes Verzeichnis" können Sie alternativ einen Bibliotheks-API-Token verwenden.',
      seafileRepoTokenTip:
        'Ein bibliotheksbezogener API-Token, der nur Zugriff auf eine bestimmte Bibliothek gewährt. ' +
        'Kann anstelle des Konto-API-Tokens für "Einzelne Bibliothek" und "Bestimmtes Verzeichnis" verwendet werden.',
      seafileRepoIdTip:
        'Die eindeutige Kennung (UUID) der SeaFile-Bibliothek. ' +
        'Sie finden diese in der Adressleiste Ihres Browsers, wenn Sie die Bibliothek in der SeaFile-Weboberflaeche öffnen. ' +
        'Beispiel: 7a9e1b3c-4d5f-6a7b-8c9d-0e1f2a3b4c5d. ' +
        'Erforderlich bei Synchronisierungsumfang "Einzelne Bibliothek" oder "Bestimmtes Verzeichnis".',
      seafileSyncPathTip:
        'Der absolute Pfad des zu synchronisierenden Ordners innerhalb der oben angegebenen Bibliothek. ' +
        'Muss mit einem Schraegstrich beginnen. ' +
        'Alle Dateien und Unterordner unter diesem Pfad werden rekursiv einbezogen. ' +
        'Beispiel: /Dokumente/Berichte. ' +
        'Wichtig: Der Ordner muss innerhalb der angegebenen Bibliothek existieren. ' +
        'Pfade ausserhalb der Bibliothek werden nicht unterstuetzt. ' +
        'Wird nur verwendet bei Synchronisierungsumfang "Bestimmtes Verzeichnis".',
      seafileIncludeSharedTip:
        'Wenn aktiviert, werden auch Bibliotheken synchronisiert, die andere Benutzer mit Ihnen geteilt haben. ' +
        'Wenn deaktiviert, werden nur Bibliotheken synchronisiert, die Ihrem Konto gehoeren. ' +
        'Gilt nur bei Synchronisierungsumfang "Gesamtes Konto".',
      seafileBatchSizeTip:
        'Anzahl der Dokumente, die pro Durchlauf verarbeitet und zurueckgegeben werden. ' +
        'Ein kleinerer Wert verbraucht weniger Arbeitsspeicher, kann aber insgesamt langsamer sein. ' +
        'Standardwert: 100.',
      jiraDescription:
        'Verbinden Sie Ihren Jira-Arbeitsbereich, um Vorgänge, Kommentare und Anhänge zu synchronisieren.',
      jiraBaseUrlTip:
        'Basis-URL Ihrer Jira-Site (z.B. https://your-domain.atlassian.net).',
      jiraProjectKeyTip:
        'Optional: Beschränken Sie die Synchronisierung auf einen einzelnen Projektschlüssel (z.B. ENG).',
      jiraJqlTip:
        'Optionaler JQL-Filter. Lassen Sie das Feld leer, um sich auf Projekt-/Zeitfilter zu verlassen.',
      jiraBatchSizeTip:
        'Maximale Anzahl von Vorgängen, die pro Batch von Jira angefordert werden.',
      jiraCommentsTip:
        'Jira-Kommentare in das generierte Markdown-Dokument aufnehmen.',
      jiraAttachmentsTip:
        'Anhänge während der Synchronisierung als separate Dokumente herunterladen.',
      jiraAttachmentSizeTip:
        'Anhänge, die größer als diese Anzahl von Bytes sind, werden übersprungen.',
      jiraLabelsTip:
        'Labels, die beim Indizieren übersprungen werden sollen (kommagetrennt).',
      jiraBlacklistTip:
        'Kommentare, deren Autoren-E-Mail mit diesen Einträgen übereinstimmt, werden ignoriert.',
      jiraScopedTokenTip:
        'Aktivieren Sie dies, wenn Sie bereichsbezogene Atlassian-Token verwenden (api.atlassian.com).',
      jiraEmailTip: 'E-Mail, die mit dem Jira-Konto/API-Token verknüpft ist.',
      jiraTokenTip:
        'API-Token generiert von https://id.atlassian.com/manage-profile/security/api-tokens.',
      jiraPasswordTip:
        'Optionales Passwort für Jira Server/Data Center-Umgebungen.',
      availableSourcesDescription: 'Wählen Sie eine Datenquelle zum Hinzufügen',
      availableSources: 'Verfügbare Quellen',
      datasourceDescription: 'Verwalten Sie Ihre Datenquellen und Verbindungen',
      save: 'Speichern',
      search: 'Suchen',
      availableModels: 'Verfügbare Modelle',
      profile: 'Profil',
      avatar: 'Avatar',
      avatarTip: 'Dies wird in Ihrem Profil angezeigt.',
      profileDescription:
        'Aktualisieren Sie hier Ihr Foto und Ihre persönlichen Daten.',
      maxTokens: 'Maximale Tokens',
      maxTokensMessage: 'Maximale Tokens sind erforderlich',
      maxTokensTip: `Die maximale Kontextgröße des Modell; ein ungültiger oder falscher Wert führt zu einem Fehler. Standardmäßig 512.`,
      maxTokensInvalidMessage:
        'Bitte geben Sie eine gültige Zahl für Maximale Tokens ein.',
      maxTokensMinMessage: 'Maximale Tokens können nicht weniger als 0 sein.',
      password: 'Passwort',
      passwordDescription:
        'Bitte geben Sie Ihr aktuelles Passwort ein, um Ihr Passwort zu ändern.',
      model: 'Modellanbieter',
      systemModelDescription:
        'Bitte schließen Sie diese Einstellungen ab, bevor Sie beginnen',
      dataSources: 'Datenquellen',
      team: 'Team',
      system: 'System',
      logout: 'Abmelden',
      api: 'API',
      username: 'Benutzername',
      usernameMessage: 'Bitte geben Sie Ihren Benutzernamen ein!',
      photo: 'Ihr Foto',
      photoDescription: 'Dies wird in Ihrem Profil angezeigt.',
      colorSchema: 'Farbschema',
      colorSchemaMessage: 'Bitte wählen Sie Ihr Farbschema!',
      colorSchemaPlaceholder: 'Wählen Sie Ihr Farbschema',
      bright: 'Hell',
      dark: 'Dunkel',
      timezone: 'Zeitzone',
      timezoneMessage: 'Bitte geben Sie Ihre Zeitzone ein!',
      timezonePlaceholder: 'Wählen Sie Ihre Zeitzone',
      email: 'E-Mail-Adresse',
      emailDescription:
        'Nach der Registrierung kann die E-Mail nicht mehr geändert werden.',
      currentPassword: 'Aktuelles Passwort',
      currentPasswordMessage: 'Bitte geben Sie Ihr Passwort ein!',
      newPassword: 'Neues Passwort',
      newPasswordMessage: 'Bitte geben Sie Ihr Passwort ein!',
      newPasswordDescription:
        'Ihr neues Passwort muss mehr als 8 Zeichen haben.',
      confirmPassword: 'Neues Passwort bestätigen',
      confirmPasswordMessage: 'Bitte bestätigen Sie Ihr Passwort!',
      confirmPasswordNonMatchMessage:
        'Die eingegebenen neuen Passwörter stimmen nicht überein!',
      cancel: 'Abbrechen',
      addedModels: 'Hinzugefügte Modelle',
      modelsToBeAdded: 'Hinzuzufügende Modelle',
      addTheModel: 'Modell hinzufügen',
      apiKey: 'API-Schlüssel',
      apiKeyMessage:
        'Bitte geben Sie den API-Schlüssel ein (für lokal bereitgestellte Modelle ignorieren Sie dies).',
      apiKeyTip:
        'Der API-Schlüssel kann durch Registrierung beim entsprechenden LLM-Anbieter erhalten werden.',
      showMoreModels: 'Mehr Modelle anzeigen',
      hideModels: 'Modelle ausblenden',
      baseUrl: 'Basis-URL',
      baseUrlTip:
        'Wenn Ihr API-Schlüssel von OpenAI stammt, ignorieren Sie dies. Andere Zwischenanbieter geben diese Basis-URL mit dem API-Schlüssel an.',
      tongyiBaseUrlTip:
        'Für chinesische Benutzer ist keine Eingabe erforderlich oder verwenden Sie https://dashscope.aliyuncs.com/compatible-mode/v1. Für internationale Benutzer verwenden Sie https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
      siliconBaseUrlTip:
        'Für chinesische Benutzer ist keine Eingabe erforderlich oder verwenden Sie https://api.siliconflow.cn/v1. Für internationale Benutzer verwenden Sie https://api.siliconflow.com/v1',
      tongyiBaseUrlPlaceholder:
        '(Nur für internationale Benutzer, bitte Hinweis beachten)',
      minimaxBaseUrlTip:
        'Nur für internationale Nutzer: https://api.minimax.io/v1 verwenden.',
      minimaxBaseUrlPlaceholder:
        '(Nur für internationale Benutzer, https://api.minimax.io/v1 eintragen)',
      modify: 'Ändern',
      systemModelSettings: 'Standardmodelle festlegen',
      chatModel: 'Chat-Modell',
      chatModelTip:
        'Das Standard-Chat-LLM, das alle neu erstellten Wissensdatenbanken verwenden werden.',
      embeddingModel: 'Embedding-Modell',
      embeddingModelTip:
        'Das Standard-Einbettungsmodell für jede neu erstellte Wissensdatenbank. Wenn Sie kein Einbettungsmodell in der Dropdown-Liste finden, prüfen Sie, ob Sie die RAGFlow Slim Edition verwenden (die keine Einbettungsmodelle enthält), oder überprüfen Sie https://ragflow.io/docs/dev/supported_models, um zu sehen, ob Ihr Modellanbieter dieses Modell unterstützt.',
      img2txtModel: 'Img2txt-Modell',
      img2txtModelTip:
        'Das Standardmodell img2txt für jede neu erstellte Wissensdatenbank. Es beschreibt ein Bild oder Video. Wenn Sie kein Modell im Dropdown-Menü finden können, überprüfen Sie https://ragflow.io/docs/dev/supported_models, um zu sehen, ob Ihr Modellanbieter dieses Modell unterstützt.',
      sequence2txtModel: 'Sequence2txt-Modell',
      sequence2txtModelTip:
        'Das Standard-ASR-Modell, das alle neu erstellten Wissensdatenbanken verwenden werden. Verwenden Sie dieses Modell, um Stimmen in entsprechenden Text zu übersetzen. Wenn Sie kein Modell im Dropdown-Menü finden können, überprüfen Sie https://ragflow.io/docs/dev/supported_models, um zu sehen, ob Ihr Modellanbieter dieses Modell unterstützt.',
      rerankModel: 'Rerank-Modell',
      rerankModelTip:
        'Das Standard-Rerank-Modell zum Reranking von Textabschnitten. Wenn Sie kein Modell im Dropdown-Menü finden, überprüfen Sie https://ragflow.io/docs/dev/supported_models, um zu sehen, ob Ihr Modellanbieter dieses Modell unterstützt.',
      ttsModel: 'TTS-Modell',
      ttsModelTip:
        'Das Standard-Text-to-Speech-Modell. Wenn Sie kein Modell im Dropdown-Menü finden, überprüfen Sie https://ragflow.io/docs/dev/supported_models, um zu sehen, ob Ihr Modellanbieter dieses Modell unterstützt.',
      workspace: 'Arbeitsbereich',
      upgrade: 'Upgrade',
      addLlmTitle: 'LLM hinzufügen',
      editLlmTitle: 'Modell {{name}} bearbeiten',
      editModel: 'Modell bearbeiten',
      modelName: 'Modellname',
      modelID: 'Modell-ID',
      modelUid: 'Modell-UID',
      modelNameMessage: 'Bitte geben Sie Ihren Modellnamen ein!',
      modelType: 'Modelltyp',
      modelTypeMessage: 'Bitte geben Sie Ihren Modelltyp ein!',
      addLlmBaseUrl: 'Basis-URL',
      baseUrlNameMessage: 'Bitte geben Sie Ihre Basis-URL ein!',
      paddleocr: {
        apiUrl: 'PaddleOCR API-URL',
        apiUrlPlaceholder:
          'Zum Beispiel: https://paddleocr-server.com/layout-parsing',
        accessToken: 'AI Studio-Zugriffstoken',
        accessTokenPlaceholder: 'Ihr AI Studio-Token (optional)',
        algorithm: 'PaddleOCR-Algorithmus',
        selectAlgorithm: 'Algorithmus auswählen',
        modelNamePlaceholder: 'Zum Beispiel: paddleocr-from-env-1',
        modelNameRequired: 'Der Modellname ist ein Pflichtfeld',
        apiUrlRequired: 'Die PaddleOCR API-URL ist ein Pflichtfeld',
      },
      vision: 'Unterstützt es Vision?',
      ollamaLink: 'Wie integriere ich {{name}}',
      FishAudioLink: 'Wie verwende ich FishAudio',
      TencentCloudLink: 'Wie verwende ich TencentCloud ASR',
      volcModelNameMessage: 'Bitte geben Sie Ihren Modellnamen ein!',
      addEndpointID: 'EndpointID des Modells',
      endpointIDMessage: 'Bitte geben Sie Ihre EndpointID des Modells ein',
      addArkApiKey: 'VOLC ARK_API_KEY',
      ArkApiKeyMessage: 'Bitte geben Sie Ihren ARK_API_KEY ein',
      bedrockModelNameMessage: 'Bitte geben Sie Ihren Modellnamen ein!',
      addBedrockEngineAK: 'ZUGRIFFSSCHLÜSSEL',
      bedrockAKMessage: 'Bitte geben Sie Ihren ZUGRIFFSSCHLÜSSEL ein',
      addBedrockSK: 'GEHEIMER SCHLÜSSEL',
      bedrockSKMessage: 'Bitte geben Sie Ihren GEHEIMEN SCHLÜSSEL ein',
      bedrockRegion: 'AWS-Region',
      bedrockRegionMessage: 'Bitte auswählen!',
      'us-east-2': 'US-Ost (Ohio)',
      'us-east-1': 'US-Ost (N. Virginia)',
      'us-west-1': 'US-West (N. Kalifornien)',
      'us-west-2': 'US-West (Oregon)',
      'af-south-1': 'Afrika (Kapstadt)',
      'ap-east-1': 'Asien-Pazifik (Hongkong)',
      'ap-south-2': 'Asien-Pazifik (Hyderabad)',
      'ap-southeast-3': 'Asien-Pazifik (Jakarta)',
      'ap-southeast-5': 'Asien-Pazifik (Malaysia)',
      'ap-southeast-4': 'Asien-Pazifik (Melbourne)',
      'ap-south-1': 'Asien-Pazifik (Mumbai)',
      'ap-northeast-3': 'Asien-Pazifik (Osaka)',
      'ap-northeast-2': 'Asien-Pazifik (Seoul)',
      'ap-southeast-1': 'Asien-Pazifik (Singapur)',
      'ap-southeast-2': 'Asien-Pazifik (Sydney)',
      'ap-east-2': 'Asien-Pazifik (Taipeh)',
      'ap-southeast-7': 'Asien-Pazifik (Thailand)',
      'ap-northeast-1': 'Asien-Pazifik (Tokio)',
      'ca-central-1': 'Kanada (Zentral)',
      'ca-west-1': 'Kanada West (Calgary)',
      'eu-central-1': 'Europa (Frankfurt)',
      'eu-west-1': 'Europa (Irland)',
      'eu-west-2': 'Europa (London)',
      'eu-south-1': 'Europa (Mailand)',
      'eu-west-3': 'Europa (Paris)',
      'eu-south-2': 'Europa (Spanien)',
      'eu-north-1': 'Europa (Stockholm)',
      'eu-central-2': 'Europa (Zürich)',
      'il-central-1': 'Israel (Tel Aviv)',
      'mx-central-1': 'Mexiko (Zentral)',
      'me-south-1': 'Naher Osten (Bahrain)',
      'me-central-1': 'Naher Osten (VAE)',
      'sa-east-1': 'Südamerika (São Paulo)',
      'us-gov-east-1': 'AWS GovCloud (US-Ost)',
      'us-gov-west-1': 'AWS GovCloud (US-West)',
      addTencentCloudSID: 'TencentCloud Secret ID',
      TencentCloudSIDMessage: 'Bitte geben Sie Ihre Secret ID ein',
      addTencentCloudSK: 'TencentCloud Secret Key',
      TencentCloudSKMessage: 'Bitte geben Sie Ihren Secret Key ein',
      SparkModelNameMessage: 'Bitte wählen Sie das Spark-Modell',
      addSparkAPIPassword: 'Spark APIPassword',
      SparkAPIPasswordMessage: 'Bitte geben Sie Ihr APIPassword ein',
      addSparkAPPID: 'Spark APP ID',
      SparkAPPIDMessage: 'Bitte geben Sie Ihre APP ID ein',
      addSparkAPISecret: 'Spark APISecret',
      SparkAPISecretMessage: 'Bitte geben Sie Ihr APISecret ein',
      addSparkAPIKey: 'Spark APIKey',
      SparkAPIKeyMessage: 'Bitte geben Sie Ihren APIKey ein',
      yiyanModelNameMessage: 'Bitte geben Sie den Modellnamen ein',
      addyiyanAK: 'yiyan API KEY',
      yiyanAKMessage: 'Bitte geben Sie Ihren API KEY ein',
      addyiyanSK: 'yiyan Secret KEY',
      yiyanSKMessage: 'Bitte geben Sie Ihren Secret KEY ein',
      FishAudioModelNameMessage:
        'Bitte geben Sie Ihrem Sprachsynthesemodell einen Namen',
      addFishAudioAK: 'Fish Audio API KEY',
      addFishAudioAKMessage: 'Bitte geben Sie Ihren API KEY ein',
      addFishAudioRefID: 'FishAudio Referenz-ID',
      addFishAudioRefIDMessage:
        'Bitte geben Sie die Referenz-ID ein (lassen Sie das Feld leer, um das Standardmodell zu verwenden).',
      GoogleModelIDMessage: 'Bitte geben Sie Ihre Modell-ID ein!',
      addGoogleProjectID: 'Projekt-ID',
      GoogleProjectIDMessage: 'Bitte geben Sie Ihre Projekt-ID ein',
      addGoogleServiceAccountKey:
        'Dienstkontoschlüssel (Lassen Sie das Feld leer, wenn Sie Anwendungsstandardanmeldedaten verwenden)',
      GoogleServiceAccountKeyMessage:
        'Bitte geben Sie den Google Cloud Dienstkontoschlüssel im base64-Format ein',
      addGoogleRegion: 'Google Cloud Region',
      GoogleRegionMessage: 'Bitte geben Sie die Google Cloud Region ein',
      modelProvidersWarn:
        'Bitte fügen Sie zuerst sowohl das Embedding-Modell als auch das LLM in <b>Einstellungen > Modellanbieter</b> hinzu. Legen Sie sie dann in "Standardmodelle festlegen" fest.',
      apiVersion: 'API-Version',
      apiVersionMessage: 'Bitte geben Sie die API-Version ein',
      add: 'Hinzufügen',
      updateDate: 'Aktualisierungsdatum',
      role: 'Rolle',
      invite: 'Einladen',
      agree: 'Akzeptieren',
      refuse: 'Ablehnen',
      teamMembers: 'Teammitglieder',
      joinedTeams: 'Beigetretene Teams',
      sureDelete:
        'Sind Sie sicher, dass Sie dieses Mitglied entfernen möchten?',
      quit: 'Verlassen',
      sureQuit:
        'Sind Sie sicher, dass Sie das Team, dem Sie beigetreten sind, verlassen möchten?',
      secretKey: 'Geheimer Schlüssel',
      publicKey: 'Öffentlicher Schlüssel',
      secretKeyMessage: 'Bitte geben Sie den geheimen Schlüssel ein',
      publicKeyMessage: 'Bitte geben Sie den öffentlichen Schlüssel ein',
      hostMessage: 'Bitte geben Sie den Host ein',
      configuration: 'Konfiguration',
      langfuseDescription:
        'Traces, Evals, Prompt-Management und Metriken zum Debuggen und Verbessern Ihrer LLM-Anwendung.',
      viewLangfuseSDocumentation: 'Langfuse-Dokumentation ansehen',
      view: 'Ansehen',
      modelsToBeAddedTooltip:
        'Wenn Ihr Modellanbieter nicht aufgeführt ist, aber behauptet, „OpenAI-kompatibel“ zu sein, wählen Sie die Karte OpenAI-API-compatible, um das/die entsprechende(n) Modell(e) hinzuzufügen.',
      mcp: 'MCP',
      mineru: {
        modelNameRequired: 'Modellname ist erforderlich',
        apiServerRequired: 'MinerU API-Server-Konfiguration ist erforderlich',
        serverUrlBackendLimit:
          'MinerU Server-URL-Adresse ist nur für das HTTP-Client-Backend verfügbar',
        apiserver: 'MinerU API-Server-Konfiguration',
        outputDir: 'MinerU Ausgabeverzeichnispfad',
        backend: 'MinerU Verarbeitungs-Backend-Typ',
        serverUrl: 'MinerU Server-URL-Adresse',
        deleteOutput: 'Ausgabedateien nach Verarbeitung löschen',
        selectBackend: 'Verarbeitungs-Backend auswählen',
        backendOptions: {
          pipeline: 'Standard-Pipeline-Verarbeitung',
          vlmTransformers: 'Vision Language Model mit Transformers',
          vlmVllmEngine: 'Vision Language Model mit vLLM Engine',
          vlmHttpClient: 'Vision Language Model über HTTP-Client',
          vlmMlxEngine: 'Vision Language Model mit MLX Engine',
          vlmVllmAsyncEngine:
            'Vision Language Model mit vLLM Async Engine (Experimentell)',
          vlmLmdeployEngine:
            'Vision Language Model mit LMDeploy Engine (Experimentell)',
        },
      },
      modelTypes: {
        chat: 'Chat',
        embedding: 'Embedding',
        rerank: 'Rerank',
        sequence2text: 'sequence2text',
        tts: 'TTS',
        image2text: 'Img2txt',
        speech2text: 'ASR',
      },
    },
    message: {
      registered: 'Registriert!',
      logout: 'Abgemeldet',
      logged: 'Angemeldet!',
      pleaseSelectChunk: 'Bitte wählen Sie einen Chunk aus!',
      registerDisabled: 'Benutzerregistrierung ist deaktiviert',
      modified: 'Geändert',
      created: 'Erstellt',
      deleted: 'Gelöscht',
      renamed: 'Umbenannt',
      operated: 'Ausgeführt',
      updated: 'Aktualisiert',
      uploaded: 'Hochgeladen',
      200: 'Der Server gibt die angeforderten Daten erfolgreich zurück.',
      201: 'Daten erfolgreich erstellt oder geändert.',
      202: 'Eine Anfrage wurde im Hintergrund in die Warteschlange gestellt (asynchrone Aufgabe).',
      204: 'Daten erfolgreich gelöscht.',
      400: 'Bei der gestellten Anfrage ist ein Fehler aufgetreten, und der Server hat keine Daten erstellt oder geändert.',
      401: 'Bitte melden Sie sich erneut an.',
      403: 'Der Benutzer ist autorisiert, aber der Zugriff ist verboten.',
      404: 'Die Anfrage wurde für einen nicht existierenden Datensatz gestellt, und der Server hat den Vorgang nicht ausgeführt.',
      406: 'Das angeforderte Format ist nicht verfügbar.',
      410: 'Die angeforderte Ressource wurde dauerhaft gelöscht und wird nicht mehr verfügbar sein.',
      413: 'Die Gesamtgröße der auf einmal hochgeladenen Dateien ist zu groß.',
      422: 'Beim Erstellen eines Objekts ist ein Validierungsfehler aufgetreten.',
      500: 'Ein Serverfehler ist aufgetreten, bitte überprüfen Sie den Server.',
      502: 'Gateway-Fehler.',
      503: 'Der Dienst ist nicht verfügbar und der Server ist vorübergehend überlastet oder wird gewartet.',
      504: 'Gateway-Timeout.',
      requestError: 'Anfragefehler',
      networkAnomalyDescription:
        'Es liegt eine Anomalie in Ihrem Netzwerk vor und Sie können keine Verbindung zum Server herstellen.',
      networkAnomaly: 'Netzwerkanomalie',
      hint: 'Hinweis',
    },
    fileManager: {
      files: 'Dateien',
      name: 'Name',
      uploadDate: 'Hochladedatum',
      knowledgeBase: 'Wissensdatenbank',
      size: 'Größe',
      action: 'Aktion',
      addToKnowledge: 'Mit Wissensdatenbank verknüpfen',
      pleaseSelect: 'Bitte auswählen',
      newFolder: 'Neuer Ordner',
      file: 'Datei',
      uploadFile: 'Datei hochladen',
      parseOnCreation: 'Bei Erstellung analysieren',
      directory: 'Verzeichnis',
      uploadTitle: 'Ziehen Sie Ihre Datei hierher, um sie hochzuladen',
      uploadDescription:
        'RAGFlow unterstützt das Hochladen von Dateien einzeln oder in Batches. Für lokal bereitgestelltes RAGFlow: Die maximale Dateigröße pro Upload beträgt 1 GB, mit einem Batch-Upload-Limit von 32 Dateien. Es gibt keine Begrenzung der Gesamtanzahl an Dateien pro Konto. Für cloud.ragflow.io: Die maximale Dateigröße pro Upload beträgt 10 MB, wobei jede Datei nicht größer als 10 MB sein darf und maximal 128 Dateien pro Konto erlaubt sind.',
      local: 'Lokale Uploads',
      s3: 'S3-Uploads',
      preview: 'Vorschau',
      fileError: 'Dateifehler',
      uploadLimit:
        'Jede Datei darf 10MB nicht überschreiten, und die Gesamtzahl der Dateien darf 128 nicht überschreiten.',
      destinationFolder: 'Zielordner',
      pleaseUploadAtLeastOneFile: 'Bitte laden Sie mindestens eine Datei hoch',
    },
    flow: {
      autoPlay: 'Audio automatisch abspielen',
      downloadFileTypeTip: 'Der herunterzuladende Dateityp',
      downloadFileType: 'Dateityp herunterladen',
      formatTypeError: 'Format- oder Typfehler',
      variableNameMessage:
        'Variablenname darf nur Buchstaben, Unterstriche und Zahlen enthalten',
      variableDescription: 'Variablenbeschreibung',
      defaultValue: 'Standardwert',
      conversationVariable: 'Konversationsvariable',
      recommended: 'Empfohlen',
      customerSupport: 'Kundensupport',
      marketing: 'Marketing',
      consumerApp: 'Verbraucher-App',
      other: 'Andere',
      ingestionPipeline: 'Dateneingabe-Pipeline',
      agents: 'Agenten',
      days: 'Tage',
      beginInput: 'Eingabe beginnen',
      ref: 'Variable',
      stockCode: 'Aktienkürzel',
      apiKeyPlaceholder:
        'IHRE_API_KEY (erhalten von https://serpapi.com/manage-api-key)',
      flowStart: 'Start',
      flowNum: 'N',
      test: 'Test',
      extractDepth: 'Extraktionstiefe',
      format: 'Format',
      basic: 'Basis',
      advanced: 'Erweitert',
      general: 'Allgemein',
      searchDepth: 'Suchtiefe',
      tavilyTopic: 'Tavily-Thema',
      maxResults: 'Maximale Ergebnisse',
      includeAnswer: 'Antwort einschließen',
      includeRawContent: 'Rohinhalt einschließen',
      includeImages: 'Bilder einschließen',
      includeImageDescriptions: 'Bildbeschreibungen einschließen',
      includeDomains: 'Domänen einschließen',
      ExcludeDomains: 'Domänen ausschließen',
      Days: 'Tage',
      comma: 'Komma',
      semicolon: 'Semikolon',
      period: 'Punkt',
      lineBreak: 'Zeilenumbruch',
      tab: 'Tabulator',
      space: 'Leerzeichen',
      delimiters: 'Trennzeichen',
      enableChildrenDelimiters:
        'Untergeordnete Chunks werden für den Abruf verwendet',
      merge: 'Zusammenführen',
      split: 'Teilen',
      script: 'Skript',
      iterationItemDescription:
        'Es repräsentiert das aktuelle Element in der Iteration, das in nachfolgenden Schritten referenziert und manipuliert werden kann.',
      guidingQuestion: 'Leitfrage',
      onFailure: 'Bei Fehler',
      userPromptDefaultValue:
        'Dies ist der Befehl, den Sie an den Agenten senden müssen.',
      search: 'Suchen',
      communication: 'Kommunikation',
      developer: 'Entwickler',
      typeCommandORsearch: 'Geben Sie einen Befehl ein oder suchen Sie...',
      builtIn: 'Eingebaut',
      ExceptionDefaultValue: 'Ausnahme-Standardwert',
      exceptionMethod: 'Ausnahmemethode',
      maxRounds: 'Maximale Reflexionsrunden',
      delayAfterError: 'Verzögerung nach Fehler',
      maxRetries: 'Maximale Wiederholungsrunden',
      advancedSettings: 'Erweiterte Einstellungen',
      addTools: 'Tools hinzufügen',
      sysPromptDefaultValue: `
      <role>
        Sie sind ein hilfreicher Assistent, ein KI-Assistent, der auf Problemlösung für den Benutzer spezialisiert ist.
        Wenn eine bestimmte Domäne angegeben ist, passen Sie Ihre Expertise an diese Domäne an; andernfalls agieren Sie als Generalist.
      </role>
      <instructions>
        1. Verstehen Sie die Anfrage des Benutzers.
        2. Zerlegen Sie sie in logische Teilaufgaben.
        3. Führen Sie jede Teilaufgabe Schritt für Schritt aus und begründen Sie transparent.
        4. Validieren Sie Genauigkeit und Konsistenz.
        5. Fassen Sie das Endergebnis klar zusammen.
      </instructions>`,
      singleLineText: 'Einzeiliger Text',
      multimodalModels: 'Multimodale Modelle',
      textOnlyModels: 'Nur-Text-Modelle',
      allModels: 'Alle Modelle',
      codeExecDescription:
        'Schreiben Sie Ihre eigene Python- oder Javascript-Logik.',
      stringTransformDescription:
        'Modifiziert Textinhalt. Unterstützt derzeit: Teilen oder Verketten von Text.',
      foundation: 'Grundlage',
      tools: 'Tools',
      dataManipulation: 'Datenmanipulation',
      flow: 'Ablauf',
      dialog: 'Dialog',
      cite: 'Zitieren',
      citeTip: 'Zitiertipp',
      name: 'Name',
      nameMessage: 'Bitte Namen eingeben',
      description: 'Beschreibung',
      descriptionMessage: 'Dies ist ein Agent für eine bestimmte Aufgabe.',
      examples: 'Beispiele',
      to: 'Zu',
      msg: 'Nachrichten',
      msgTip:
        'Geben Sie den Variableninhalt der vorgelagerten Komponente oder den von Ihnen eingegebenen Text aus.',
      messagePlaceholder: 'Nachricht',
      messageMsg: 'Bitte Nachricht eingeben oder dieses Feld löschen.',
      addField: 'Option hinzufügen',
      addMessage: 'Nachricht hinzufügen',
      loop: 'Schleife',
      loopDescription:
        'Schleife ist die Obergrenze der Anzahl der Durchläufe der aktuellen Komponente. Wenn die Anzahl der Durchläufe den Wert der Schleife überschreitet, bedeutet dies, dass die Komponente die aktuelle Aufgabe nicht abschließen kann. Bitte optimieren Sie den Agenten neu',
      loopTip:
        'Schleife ist die Obergrenze der Anzahl der Durchläufe der aktuellen Komponente. Wenn die Anzahl der Durchläufe den Wert der Schleife überschreitet, bedeutet dies, dass die Komponente die aktuelle Aufgabe nicht abschließen kann. Bitte optimieren Sie den Agenten neu',
      exitLoop: 'Schleife verlassen',
      exitLoopDescription: `Äquivalent zu "break". Dieser Knoten hat keine Konfigurationselemente. Wenn der Schleifenkörper diesen Knoten erreicht, endet die Schleife.`,
      loopVariables: 'Schleifenvariablen',
      maximumLoopCount: 'Maximale Schleifenanzahl',
      loopTerminationCondition: 'Schleifenabbruchbedingung',
      yes: 'Ja',
      no: 'Nein',
      key: 'Schlüssel',
      componentId: 'Komponenten-ID',
      add: 'Hinzufügen',
      operation: 'Operation',
      run: 'Ausführen',
      save: 'Speichern',
      title: 'ID:',
      beginDescription: 'Hier beginnt der Ablauf.',
      answerDescription:
        'Eine Komponente, die als Schnittstelle zwischen Mensch und Bot dient, Benutzereingaben empfängt und die Antworten des Agenten anzeigt.',
      retrievalDescription:
        'Eine Komponente, die Informationen aus bestimmten Wissensdatenbanken (Datensätzen) abruft. Stellen Sie sicher, dass die von Ihnen ausgewählten Wissensdatenbanken dasselbe Embedding-Modell verwenden.',
      generateDescription:
        'Eine Komponente, die das LLM auffordert, Antworten zu generieren. Stellen Sie sicher, dass der Prompt korrekt eingestellt ist.',
      categorizeDescription:
        'Eine Komponente, die das LLM verwendet, um Benutzereingaben in vordefinierte Kategorien zu klassifizieren. Stellen Sie sicher, dass Sie für jede Kategorie den Namen, die Beschreibung und Beispiele sowie die entsprechende nächste Komponente angeben.',
      relevantDescription:
        'Eine Komponente, die das LLM verwendet, um zu beurteilen, ob die vorgelagerte Ausgabe für die neueste Anfrage des Benutzers relevant ist. Stellen Sie sicher, dass Sie die nächste Komponente für jedes Beurteilungsergebnis angeben.',
      rewriteQuestionDescription:
        'Eine Komponente, die eine Benutzeranfrage aus der Interaktionskomponente basierend auf dem Kontext vorheriger Dialoge umformuliert.',
      messageDescription:
        'Eine Komponente, die eine statische Nachricht sendet. Wenn mehrere Nachrichten bereitgestellt werden, wählt sie zufällig eine zum Senden aus. Stellen Sie sicher, dass ihr nachgelagerter Bereich "Interact" ist, die Schnittstellenkomponente.',
      keywordDescription:
        'Eine Komponente, die die Top-N-Suchergebnisse aus der Benutzereingabe abruft. Stellen Sie sicher, dass der TopN-Wert vor der Verwendung richtig eingestellt ist.',
      switchDescription:
        'Eine Komponente, die Bedingungen basierend auf der Ausgabe vorheriger Komponenten auswertet und den Ausführungsfluss entsprechend lenkt. Sie ermöglicht komplexe Verzweigungslogik, indem Fälle definiert und Aktionen für jeden Fall oder Standardaktionen festgelegt werden, wenn keine Bedingungen erfüllt sind.',
      wikipediaDescription:
        'Eine Komponente, die auf wikipedia.org sucht und mit TopN die Anzahl der Suchergebnisse angibt. Sie ergänzt die vorhandenen Wissensdatenbanken.',
      promptText:
        'Bitte fassen Sie die folgenden Absätze zusammen. Seien Sie vorsichtig mit den Zahlen, erfinden Sie nichts. Absätze wie folgt:\n{input}\nDas oben ist der Inhalt, den Sie zusammenfassen müssen.',
      createGraph: 'Agenten erstellen',
      createFromTemplates: 'Aus Vorlagen erstellen',
      retrieval: 'Abruf',
      generate: 'Generieren',
      answer: 'Interagieren',
      categorize: 'Kategorisieren',
      rewriteQuestion: 'Umschreiben',
      rewrite: 'Umschreiben',
      begin: 'Beginn',
      message: 'Nachricht',
      blank: 'Leer',
      createFromNothing: 'Erstellen Sie Ihren Agenten von Grund auf',
      addItem: 'Element hinzufügen',
      addSubItem: 'Unterelement hinzufügen',
      nameRequiredMsg: 'Name ist erforderlich',
      nameRepeatedMsg: 'Der Name darf nicht wiederholt werden',
      keywordExtract: 'Schlüsselwort',
      keywordExtractDescription:
        'Eine Komponente, die Schlüsselwörter aus einer Benutzeranfrage extrahiert, wobei Top N die Anzahl der zu extrahierenden Schlüsselwörter angibt.',
      baidu: 'Baidu',
      baiduDescription:
        'Eine Komponente, die auf baidu.com sucht und mit TopN die Anzahl der Suchergebnisse angibt. Sie ergänzt die vorhandenen Wissensdatenbanken.',
      duckDuckGo: 'DuckDuckGo',
      duckDuckGoDescription:
        'Eine Komponente, die auf duckduckgo.com sucht und Ihnen ermöglicht, die Anzahl der Suchergebnisse mit TopN anzugeben. Sie ergänzt die vorhandenen Wissensdatenbanken.',
      searXNG: 'SearXNG',
      searXNGDescription:
        'Eine Komponente, die auf https://searxng.org/ sucht und Ihnen ermöglicht, die Anzahl der Suchergebnisse mit TopN anzugeben. Sie ergänzt die vorhandenen Wissensdatenbanken.',
      pdfGenerator: 'Dokumentengenerator',
      pDFGenerator: 'Dokumentengenerator',
      pdfGeneratorDescription: `Eine Komponente, die Dokumente (PDF, DOCX, TXT) aus markdown-formatierten Inhalten mit anpassbarem Stil, Bildern und Tabellen generiert. Unterstützt: **fett**, *kursiv*, # Überschriften, - Listen, Tabellen mit | Syntax.`,
      pDFGeneratorDescription: `Eine Komponente, die Dokumente (PDF, DOCX, TXT) aus markdown-formatierten Inhalten mit anpassbarem Stil, Bildern und Tabellen generiert. Unterstützt: **fett**, *kursiv*, # Überschriften, - Listen, Tabellen mit | Syntax.`,
      subtitle: 'Untertitel',
      logoImage: 'Logo-Bild',
      logoPosition: 'Logo-Position',
      logoWidth: 'Logo-Breite',
      logoHeight: 'Logo-Höhe',
      fontFamily: 'Schriftfamilie',
      fontSize: 'Schriftgröße',
      titleFontSize: 'Titel-Schriftgröße',
      pageSize: 'Seitengröße',
      orientation: 'Ausrichtung',
      marginTop: 'Oberer Rand',
      marginBottom: 'Unterer Rand',
      filename: 'Dateiname',
      outputDirectory: 'Ausgabeverzeichnis',
      addPageNumbers: 'Seitenzahlen hinzufügen',
      addTimestamp: 'Zeitstempel hinzufügen',
      watermarkText: 'Wasserzeichentext',
      channel: 'Kanal',
      channelTip:
        'Führt eine Textsuche oder Nachrichtensuche für die Eingabe der Komponente durch',
      text: 'Text',
      news: 'Nachrichten',
      messageHistoryWindowSize: 'Nachrichtenfenstergröße',
      messageHistoryWindowSizeTip:
        'Die Fenstergröße des für das LLM sichtbaren Gesprächsverlaufs. Größer ist besser, aber achten Sie auf das maximale Token-Limit des LLM.',
      wikipedia: 'Wikipedia',
      pubMed: 'PubMed',
      pubMedDescription:
        'Eine Komponente, die auf https://pubmed.ncbi.nlm.nih.gov/ sucht und Ihnen ermöglicht, die Anzahl der Suchergebnisse mit TopN anzugeben. Sie ergänzt die vorhandenen Wissensdatenbanken.',
      email: 'E-Mail',
      emailTip:
        'E-Mail ist ein Pflichtfeld. Sie müssen hier eine E-Mail-Adresse eingeben.',
      arXiv: 'ArXiv',
      arXivDescription:
        'Eine Komponente, die auf https://arxiv.org/ sucht und Ihnen ermöglicht, die Anzahl der Suchergebnisse mit TopN anzugeben. Sie ergänzt die vorhandenen Wissensdatenbanken.',
      sortBy: 'Sortieren nach',
      submittedDate: 'Einreichungsdatum',
      lastUpdatedDate: 'Letztes Aktualisierungsdatum',
      relevance: 'Relevanz',
      google: 'Google',
      googleDescription:
        'Eine Komponente, die auf https://www.google.com/ sucht und Ihnen ermöglicht, die Anzahl der Suchergebnisse mit TopN anzugeben. Sie ergänzt die vorhandenen Wissensdatenbanken. Bitte beachten Sie, dass hierfür ein API-Schlüssel von serpapi.com erforderlich ist.',
      bing: 'Bing',
      bingDescription:
        'Eine Komponente, die auf https://www.bing.com/ sucht und Ihnen ermöglicht, die Anzahl der Suchergebnisse mit TopN anzugeben. Sie ergänzt die vorhandenen Wissensdatenbanken. Bitte beachten Sie, dass hierfür ein API-Schlüssel von microsoft.com erforderlich ist.',
      apiKey: 'API-SCHLÜSSEL',
      country: 'Land & Region',
      language: 'Sprache',
      googleScholar: 'Google Scholar',
      googleScholarDescription:
        'Eine Komponente, die auf https://scholar.google.com/ sucht. Sie können Top N verwenden, um die Anzahl der Suchergebnisse anzugeben.',
      yearLow: 'Jahr Minimum',
      yearHigh: 'Jahr Maximum',
      patents: 'Patente',
      data: 'Daten',
      deepL: 'DeepL',
      deepLDescription:
        'Eine Komponente, die spezialisierte Übersetzungen von https://www.deepl.com/ abruft.',
      authKey: 'Authentifizierungsschlüssel',
      sourceLang: 'Quellsprache',
      targetLang: 'Zielsprache',
      gitHub: 'GitHub',
      gitHubDescription:
        'Eine Komponente, die nach Repositories auf https://github.com/ sucht. Sie können Top N verwenden, um die Anzahl der Suchergebnisse anzugeben.',
      baiduFanyi: 'BaiduFanyi',
      baiduFanyiDescription:
        'Eine Komponente, die spezialisierte Übersetzungen von https://fanyi.baidu.com/ abruft.',
      appid: 'App-ID',
      secretKey: 'Geheimer Schlüssel',
      domain: 'Domäne',
      transType: 'Übersetzungstyp',
      baiduSecretKeyOptions: {
        translate: 'Allgemeine Übersetzung',
        fieldtranslate: 'Fachübersetzung',
      },
      baiduDomainOptions: {
        it: 'Informationstechnologie',
        finance: 'Finanzen und Wirtschaft',
        machinery: 'Maschinenbau',
        senimed: 'Biomedizin',
        novel: 'Online-Literatur',
        academic: 'Wissenschaftliche Arbeit',
        aerospace: 'Luft- und Raumfahrt',
        wiki: 'Geistes- und Sozialwissenschaften',
        news: 'Nachrichten und Informationen',
        law: 'Gesetze und Vorschriften',
        contract: 'Vertrag',
      },
      baiduSourceLangOptions: {
        auto: 'Automatisch erkennen',
        zh: 'Chinesisch',
        en: 'Englisch',
        yue: 'Kantonesisch',
        wyw: 'Klassisches Chinesisch',
        jp: 'Japanisch',
        kor: 'Koreanisch',
        fra: 'Französisch',
        spa: 'Spanisch',
        th: 'Thailändisch',
        ara: 'Arabisch',
        ru: 'Russisch',
        pt: 'Portugiesisch',
        de: 'Deutsch',
        it: 'Italienisch',
        el: 'Griechisch',
        nl: 'Niederländisch',
        pl: 'Polnisch',
        bul: 'Bulgarisch',
        est: 'Estnisch',
        dan: 'Dänisch',
        fin: 'Finnisch',
        cs: 'Tschechisch',
        rom: 'Rumänisch',
        slo: 'Slowenisch',
        swe: 'Schwedisch',
        hu: 'Ungarisch',
        cht: 'Traditionelles Chinesisch',
        vie: 'Vietnamesisch',
      },
      qWeather: 'QWeather',
      qWeatherDescription:
        'Eine Komponente, die Wetterinformationen wie Temperatur und Luftqualität von https://www.qweather.com/ abruft.',
      lang: 'Sprache',
      type: 'Typ',
      webApiKey: 'Web-API-Schlüssel',
      userType: 'Benutzertyp',
      timePeriod: 'Zeitraum',
      qWeatherLangOptions: {
        zh: 'Vereinfachtes Chinesisch',
        'zh-hant': 'Traditionelles Chinesisch',
        en: 'Englisch',
        de: 'Deutsch',
        es: 'Spanisch',
        fr: 'Französisch',
        it: 'Italienisch',
        ja: 'Japanisch',
        ko: 'Koreanisch',
        ru: 'Russisch',
        hi: 'Hindi',
        th: 'Thailändisch',
        ar: 'Arabisch',
        pt: 'Portugiesisch',
        bn: 'Bengalisch',
        ms: 'Malaiisch',
        nl: 'Niederländisch',
        el: 'Griechisch',
        la: 'Lateinisch',
        sv: 'Schwedisch',
        id: 'Indonesisch',
        pl: 'Polnisch',
        tr: 'Türkisch',
        cs: 'Tschechisch',
        et: 'Estnisch',
        vi: 'Vietnamesisch',
        fil: 'Philippinisch',
        fi: 'Finnisch',
        he: 'Hebräisch',
        is: 'Isländisch',
        nb: 'Norwegisch',
      },
      qWeatherTypeOptions: {
        weather: 'Wettervorhersage',
        indices: 'Wetter-Lebensindex',
        airquality: 'Luftqualität',
      },
      qWeatherUserTypeOptions: {
        free: 'Kostenloser Abonnent',
        paid: 'Zahlender Abonnent',
      },
      qWeatherTimePeriodOptions: {
        now: 'Jetzt',
        '3d': '3 Tage',
        '7d': '7 Tage',
        '10d': '10 Tage',
        '15d': '12 Tage',
        '30d': '30 Tage',
      },
      publish: 'API',
      exeSQL: 'ExeSQL',
      exeSQLDescription:
        'Eine Komponente, die SQL-Abfragen in einer relationalen Datenbank ausführt und Abfragen von MySQL, PostgreSQL oder MariaDB unterstützt.',
      dbType: 'Datenbanktyp',
      database: 'Datenbank',
      username: 'Benutzername',
      host: 'Host',
      port: 'Port',
      password: 'Passwort',
      switch: 'Schalter',
      logicalOperator: 'Logischer Operator',
      switchOperatorOptions: {
        equal: 'Gleich',
        notEqual: 'Ungleich',
        gt: 'Größer als',
        ge: 'Größer gleich',
        lt: 'Kleiner als',
        le: 'Kleiner gleich',
        contains: 'Enthält',
        notContains: 'Enthält nicht',
        startWith: 'Beginnt mit',
        endWith: 'Endet mit',
        empty: 'Ist leer',
        notEmpty: 'Nicht leer',
        in: 'In',
        notIn: 'Nicht in',
        is: 'Ist',
        isNot: 'Ist nicht',
      },
      switchLogicOperatorOptions: {
        and: 'UND',
        or: 'ODER',
      },
      operator: 'Operator',
      value: 'Wert',
      useTemplate: 'Diese Vorlage verwenden',
      wenCai: 'WenCai',
      queryType: 'Abfragetyp',
      wenCaiDescription:
        'Eine Komponente, die Finanzinformationen, einschließlich Aktienkursen und Finanzierungsnachrichten, von einer Vielzahl von Finanzwebsites abruft.',
      wenCaiQueryTypeOptions: {
        stock: 'Aktie',
        zhishu: 'Index',
        fund: 'Fonds',
        hkstock: 'Hongkong-Aktien',
        usstock: 'US-Aktienmarkt',
        threeboard: 'Neuer OTC-Markt',
        conbond: 'Wandelanleihe',
        insurance: 'Versicherung',
        futures: 'Futures',
        lccp: 'Finanzierung',
        foreign_exchange: 'Fremdwährung',
      },
      akShare: 'AkShare',
      akShareDescription:
        'Eine Komponente, die Nachrichten über Aktien von https://www.eastmoney.com/ abruft.',
      yahooFinance: 'YahooFinance',
      yahooFinanceDescription:
        'Eine Komponente, die Informationen über ein börsennotiertes Unternehmen anhand seines Tickersymbols abfragt.',
      crawler: 'Web-Crawler',
      crawlerDescription:
        'Eine Komponente, die HTML-Quellcode von einer angegebenen URL crawlt.',
      proxy: 'Proxy',
      crawlerResultOptions: {
        html: 'Html',
        markdown: 'Markdown',
        content: 'Inhalt',
      },
      extractType: 'Extraktionstyp',
      info: 'Info',
      history: 'Verlauf',
      financials: 'Finanzen',
      balanceSheet: 'Bilanz',
      cashFlowStatement: 'Kapitalflussrechnung',
      jin10: 'Jin10',
      jin10Description:
        'Eine Komponente, die Finanzinformationen von der Jin10 Open Platform abruft, einschließlich Nachrichtenaktualisierungen, Kalendern, Kursen und Referenzen.',
      flashType: 'Flash-Typ',
      filter: 'Filter',
      contain: 'Enthalten',
      calendarType: 'Kalendertyp',
      calendarDatashape: 'Kalender-Datenform',
      symbolsDatatype: 'Symboldatentyp',
      symbolsType: 'Symboltyp',
      jin10TypeOptions: {
        flash: 'Schnellnachrichten',
        calendar: 'Kalender',
        symbols: 'Kurse',
        news: 'Referenz',
      },
      jin10FlashTypeOptions: {
        '1': 'Marktnachrichten',
        '2': 'Futures-Nachrichten',
        '3': 'US-Hongkong-Nachrichten',
        '4': 'A-Aktien-Nachrichten',
        '5': 'Rohstoff- und Devisennachrichten',
      },
      jin10CalendarTypeOptions: {
        cj: 'Makroökonomischer Datenkalender',
        qh: 'Futures-Kalender',
        hk: 'Hongkong-Aktienmarktkalender',
        us: 'US-Aktienmarktkalender',
      },
      jin10CalendarDatashapeOptions: {
        data: 'Daten',
        event: 'Ereignis',
        holiday: 'Feiertag',
      },
      jin10SymbolsTypeOptions: {
        GOODS: 'Rohstoffkurse',
        FOREX: 'Devisenkurse',
        FUTURE: 'Internationale Marktkurse',
        CRYPTO: 'Kryptowährungskurse',
      },
      jin10SymbolsDatatypeOptions: {
        symbols: 'Rohstoffliste',
        quotes: 'Aktuelle Marktkurse',
      },
      concentrator: 'Konzentrator',
      concentratorDescription:
        'Eine Komponente, die die Ausgabe der vorgelagerten Komponente empfängt und als Eingabe an die nachgelagerten Komponenten weitergibt.',
      tuShare: 'TuShare',
      tuShareDescription:
        'Eine Komponente, die Finanznahrichten-Kurzmeldungen von führenden Finanzwebsites abruft und bei Branchen- und quantitativer Forschung hilft.',
      tuShareSrcOptions: {
        sina: 'Sina',
        wallstreetcn: 'wallstreetcn',
        '10jqka': 'Straight Flush',
        eastmoney: 'Eastmoney',
        yuncaijing: 'YUNCAIJING',
        fenghuang: 'FENGHUANG',
        jinrongjie: 'JRJ',
      },
      token: 'Token',
      src: 'Quelle',
      startDate: 'Startdatum',
      endDate: 'Enddatum',
      keyword: 'Schlüsselwort',
      note: 'Notiz',
      noteDescription: 'Notiz',
      notePlaceholder: 'Bitte geben Sie eine Notiz ein',
      invoke: 'Aufrufen',
      invokeDescription:
        'Eine Komponente, die Remote-Dienste aufrufen kann und dabei die Ausgaben anderer Komponenten oder Konstanten als Eingaben verwendet.',
      url: 'URL',
      method: 'Methode',
      timeout: 'Zeitüberschreitung',
      headers: 'Header',
      cleanHtml: 'HTML bereinigen',
      cleanHtmlTip:
        'Wenn die Antwort im HTML-Format vorliegt und nur der Hauptinhalt gewünscht wird, schalten Sie dies bitte ein.',
      invalidUrl:
        'Muss eine gültige URL oder eine URL mit Variablenplatzhaltern im Format {Variablenname} oder {Komponente@Variable} sein',
      reference: 'Referenz',
      input: 'Eingabe',
      output: 'Ausgabe',
      parameter: 'Parameter',
      howUseId: 'Wie verwendet man die Agenten-ID?',
      content: 'Inhalt',
      operationResults: 'Operationsergebnisse',
      autosaved: 'Automatisch gespeichert',
      optional: 'Optional',
      pasteFileLink: 'Dateilink einfügen',
      testRun: 'Testlauf',
      template: 'Vorlage',
      templateDescription:
        'Eine Komponente, die die Ausgabe anderer Komponenten formatiert. 1. Unterstützt Jinja2-Vorlagen, konvertiert zuerst die Eingabe in ein Objekt und rendert dann die Vorlage, 2. Behält gleichzeitig die ursprüngliche Methode der Verwendung von {parameter} Zeichenkettenersetzung bei',
      emailComponent: 'E-Mail',
      emailDescription: 'Sendet eine E-Mail an eine angegebene Adresse.',
      smtpServer: 'SMTP-Host',
      smtpPort: 'SMTP-Port',
      senderEmail: 'Absenderadresse (From)',
      smtpUsername: 'SMTP-Anmeldebenutzername',
      authCode: 'SMTP-Passwort/App-Passwort',
      senderName: 'Anzeigename des Absenders',
      toEmail: 'Empfänger-E-Mail',
      ccEmail: 'CC-E-Mail',
      emailSubject: 'Betreff',
      emailContent: 'Inhalt',
      smtpServerRequired: 'Bitte geben Sie die SMTP-Serveradresse ein',
      senderEmailRequired: 'Bitte geben Sie die Absender-E-Mail ein',
      authCodeRequired: 'Bitte geben Sie den Autorisierungscode ein',
      toEmailRequired: 'Bitte geben Sie die Empfänger-E-Mail ein',
      emailContentRequired: 'Bitte geben Sie den E-Mail-Inhalt ein',
      emailSentSuccess: 'E-Mail erfolgreich gesendet',
      emailSentFailed: 'E-Mail konnte nicht gesendet werden',
      dynamicParameters: 'Dynamische Parameter',
      jsonFormatTip:
        'Die vorgelagerte Komponente sollte einen JSON-String im folgenden Format bereitstellen:',
      toEmailTip: 'to_email: Empfänger-E-Mail (Erforderlich)',
      ccEmailTip: 'cc_email: CC-E-Mail (Optional)',
      subjectTip: 'subject: E-Mail-Betreff (Optional)',
      contentTip: 'content: E-Mail-Inhalt (Optional)',
      jsonUploadTypeErrorMessage: 'Bitte laden Sie eine JSON-Datei hoch',
      jsonUploadContentErrorMessage: 'JSON-Dateifehler',
      iteration: 'Iteration',
      iterationDescription:
        'Diese Komponente teilt zunächst die Eingabe durch "Trennzeichen" in ein Array auf. Führt die gleichen Operationsschritte nacheinander für die Elemente im Array aus, bis alle Ergebnisse ausgegeben sind, was als Aufgaben-Batch-Prozessor verstanden werden kann.\n\nZum Beispiel kann innerhalb des Iterationsknotens für lange Textübersetzungen, wenn der gesamte Inhalt in den LLM-Knoten eingegeben wird, das Limit für eine einzelne Konversation erreicht werden. Der vorgelagerte Knoten kann den langen Text zuerst in mehrere Fragmente aufteilen und mit dem Iterationsknoten zusammenarbeiten, um eine Batch-Übersetzung für jedes Fragment durchzuführen, um zu vermeiden, dass das LLM-Nachrichtenlimit für eine einzelne Konversation erreicht wird.',
      delimiterTip:
        'Dieses Trennzeichen wird verwendet, um den Eingabetext in mehrere Textstücke aufzuteilen, von denen jedes als Eingabeelement jeder Iteration ausgeführt wird.',
      delimiterOptions: {
        comma: 'Komma',
        lineBreak: 'Zeilenumbruch',
        tab: 'Tabulator',
        underline: 'Unterstrich',
        diagonal: 'Schrägstrich',
        minus: 'Bindestrich',
        semicolon: 'Semikolon',
      },
      addVariable: 'Variable hinzufügen',
      variableSettings: 'Variableneinstellungen',
      systemPrompt: 'System-Prompt',
      userPrompt: 'Benutzer-Prompt',
      addCategory: 'Kategorie hinzufügen',
      categoryName: 'Kategoriename',
      nextStep: 'Nächster Schritt',
      variableExtractDescription:
        'Benutzerinformationen in globale Variable extrahieren',
      variableExtract: 'Variablen',
      variables: 'Variablen',
      variablesTip: `Setzen Sie die klare JSON-Schlüsselvariable mit einem leeren Wert. z.B.
      {
        "UserCode":"",
        "NumberPhone":""
      }`,
      datatype: 'MIME-Typ der HTTP-Anfrage',
      insertVariableTip: 'Eingabe / Variablen einfügen',
      historyVersion: 'Versionsverlauf',
      version: {
        created: 'Erstellt',
        details: 'Versionsdetails',
        dsl: 'DSL',
        download: 'Herunterladen',
        version: 'Version',
        select: 'Keine Version ausgewählt',
      },
      setting: 'Einstellungen',
      settings: {
        agentSetting: 'Agenteneinstellungen',
        title: 'Titel',
        description: 'Beschreibung',
        upload: 'Hochladen',
        photo: 'Foto',
        permissions: 'Berechtigungen',
        permissionsTip:
          'Sie können hier die Berechtigungen der Teammitglieder festlegen.',
        me: 'Ich',
        team: 'Team',
      },
      noMoreData: 'Keine weiteren Daten',
      searchAgentPlaceholder: 'Agent suchen',
      footer: {
        profile: 'Alle Rechte vorbehalten @ React',
      },
      layout: {
        file: 'Datei',
        knowledge: 'Wissen',
        chat: 'Chat',
      },
      prompt: 'Prompt',
      promptTip:
        'Verwenden Sie den Systemprompt, um die Aufgabe für das LLM zu beschreiben, festzulegen, wie es antworten soll, und andere verschiedene Anforderungen zu skizzieren. Der Systemprompt wird oft in Verbindung mit Schlüsseln (Variablen) verwendet, die als verschiedene Dateninputs für das LLM dienen. Verwenden Sie einen Schrägstrich `/` oder die (x)-Schaltfläche, um die zu verwendenden Schlüssel anzuzeigen.',
      promptMessage: 'Prompt ist erforderlich',
      infor: 'Informationslauf',
      knowledgeBasesTip:
        'Wählen Sie die Wissensdatenbanken aus, die mit diesem Chat-Assistenten verknüpft werden sollen, oder wählen Sie unten Variablen aus, die Wissensdatenbank-IDs enthalten.',
      knowledgeBaseVars: 'Wissensdatenbank-Variablen',
      code: 'Code',
      codeDescription:
        'Es ermöglicht Entwicklern, benutzerdefinierte Python-Logik zu schreiben.',
      dataOperations: 'Datenoperationen',
      dataOperationsDescription:
        'Führen Sie verschiedene Operationen auf einem Datenobjekt aus.',
      listOperations: 'Listenoperationen',
      listOperationsDescription: 'Führen Sie Operationen auf einer Liste aus.',
      variableAssigner: 'Variablenzuweiser',
      variableAssignerDescription:
        'Diese Komponente führt Operationen auf Datenobjekten aus, einschließlich Extrahieren, Filtern und Bearbeiten von Schlüsseln und Werten in den Daten.',
      variableAggregator: 'Variablenaggregator',
      variableAggregatorDescription: `
Dieser Prozess aggregiert Variablen aus mehreren Zweigen in eine einzelne Variable, um eine einheitliche Konfiguration für nachgelagerte Knoten zu erreichen.`,
      inputVariables: 'Eingabevariablen',
      runningHintText: 'läuft...🕞',
      openingSwitch: 'Eröffnungsschalter',
      openingCopy: 'Begrüßungstext',
      openingSwitchTip:
        'Ihre Benutzer werden diese Willkommensnachricht zu Beginn sehen.',
      modeTip: 'Der Modus definiert, wie der Workflow initiiert wird.',
      mode: 'Modus',
      conversational: 'Konversationell',
      task: 'Aufgabe',
      beginInputTip:
        'Hier definierte Eingabeparameter können von Komponenten im nachgelagerten Workflow abgerufen werden.',
      query: 'Abfragevariablen',
      switchPromptMessage:
        'Die Prompt-Wörter werden geändert. Bitte bestätigen Sie, ob Sie die vorhandenen Prompt-Wörter verwerfen möchten?',
      queryRequired: 'Abfrage ist erforderlich',
      queryTip: 'Wählen Sie die Variable, die Sie verwenden möchten',
      agent: 'Agent',
      addAgent: 'Agent hinzufügen',
      agentDescription:
        'Erstellt Agentenkomponenten, die mit Schlussfolgerung, Tool-Nutzung und Multi-Agenten-Kollaboration ausgestattet sind.',
      maxRecords: 'Maximale Datensätze',
      createAgent: 'Agenten-Flow',
      stringTransform: 'Textverarbeitung',
      userFillUp: 'Auf Antwort warten',
      userFillUpDescription:
        'Pausiert den Workflow und wartet auf die Nachricht des Benutzers, bevor er fortfährt.',
      codeExec: 'Code',
      tavilySearch: 'Tavily-Suche',
      tavilySearchDescription: 'Suchergebnisse über Tavily-Dienst.',
      tavilyExtract: 'Tavily-Extrakt',
      tavilyExtractDescription: 'Tavily Extrakt',
      log: 'Log',
      management: 'Verwaltung',
      import: 'Importieren',
      export: 'Exportieren',
      seconds: 'Sekunden',
      subject: 'Betreff',
      tag: 'Tag',
      tagPlaceholder: 'Bitte Tag eingeben',
      descriptionPlaceholder: 'Bitte Beschreibung eingeben',
      line: 'Einzeiliger Text',
      paragraph: 'Absatztext',
      options: 'Dropdown-Optionen',
      file: 'Datei-Upload',
      integer: 'Zahl',
      boolean: 'Boolesch',
      logTimeline: {
        begin: 'Bereit zum Starten',
        agent: 'Agent denkt nach',
        userFillUp: 'Warte auf Sie',
        retrieval: 'Schlage Wissen nach',
        message: 'Agent sagt',
        awaitResponse: 'Warte auf Sie',
        switch: 'Wähle den besten Pfad',
        iteration: 'Batch-Verarbeitung',
        categorize: 'Kategorisiere Informationen',
        code: 'Führe ein schnelles Skript aus',
        textProcessing: 'Räume Text auf',
        tavilySearch: 'Suche im Web',
        tavilyExtract: 'Lese die Seite',
        exeSQL: 'Frage Datenbank ab',
        google: 'Suche im Web',
        wikipedia: 'Suche Wikipedia',
        googleScholar: 'Akademische Suche',
        gitHub: 'Suche GitHub',
        email: 'Sende E-Mail',
        httpRequest: 'Rufe eine API auf',
        wenCai: 'Frage Finanzdaten ab',
      },
      goto: 'Fehlerzweig',
      comment: 'Standardwert',
      sqlStatement: 'SQL-Anweisung',
      sqlStatementTip:
        'Schreiben Sie hier Ihre SQL-Abfrage. Sie können Variablen, rohes SQL oder beides unter Verwendung der Variablensyntax mischen.',
      frameworkPrompts: 'Framework',
      release: 'Veröffentlichen',
      createFromBlank: 'Leer erstellen',
      createFromTemplate: 'Aus Vorlage erstellen',
      importJsonFile: 'JSON-Datei importieren',
      ceateAgent: 'Agenten-Flow',
      createPipeline: ' Dateneingabe-Pipeline',
      chooseAgentType: 'Agententyp wählen',
      parser: 'Parser',
      parserDescription:
        'Extrahiert Rohtext und Struktur aus Dateien für die nachgelagerte Verarbeitung.',
      tokenizer: 'Indexer',
      tokenizerRequired: 'Bitte fügen Sie zuerst den Indexer-Knoten hinzu',
      tokenizerDescription:
        'Transformiert Text in die erforderliche Datenstruktur (z.B. Vektoreinbettungen für die Embedding-Suche) abhängig von der gewählten Suchmethode.',
      splitter: 'Token',
      splitterDescription:
        'Teilt Text in Chunks nach Token-Länge mit optionalen Trennzeichen und Überlappung.',
      hierarchicalMergerDescription:
        'Teilt Dokumente in Abschnitte nach Titelhierarchie mit Regex-Regeln für feinere Kontrolle.',
      hierarchicalMerger: 'Titel',
      extractor: 'Transformer',
      extractorDescription:
        'Verwendet ein LLM, um strukturierte Erkenntnisse aus Dokument-Chunks zu extrahieren – wie Zusammenfassungen, Klassifizierungen usw.',
      outputFormat: 'Ausgabeformat',
      fileFormats: 'Dateityp',
      fileFormatOptions: {
        pdf: 'PDF',
        spreadsheet: 'Tabellenkalkulation',
        image: 'Bild',
        email: 'E-Mail',
        'text&markdown': 'Text & Markup',
        word: 'Word',
        slides: 'PPTX',
        audio: 'Audio',
        video: 'Video',
      },
      fields: 'Feld',
      addParser: 'Parser hinzufügen',
      hierarchy: 'Hierarchie',
      regularExpressions: 'Reguläre Ausdrücke',
      overlappedPercent: 'Chunk-Überlappung (%)',
      searchMethod: 'Suchmethode',
      searchMethodTip: `Definiert, wie der Inhalt durchsucht werden kann — durch Volltext, Embedding oder beides.
Der Indexer speichert den Inhalt in den entsprechenden Datenstrukturen für die ausgewählten Methoden.`,
      parserMethod: 'PDF-Parser',
      tableResultType: 'Tabellenergebnistyp',
      markdownImageResponseType: 'Markdown-Bildantworttyp',
      systemPromptPlaceholder:
        'System-Prompt für Bildanalyse eingeben, wenn leer, wird der Systemstandardwert verwendet',
      exportJson: 'JSON exportieren',
      viewResult: 'Ergebnis anzeigen',
      running: 'Läuft',
      summary: 'Zusammenfassung',
      keywords: 'Schlüsselwörter',
      questions: 'Fragen',
      metadata: 'Metadaten',
      toc: 'Inhaltsverzeichnis',
      fieldName: 'Ergebnisziel',
      prompts: {
        system: {
          keywords: `Rolle
Du bist ein Textanalysator.

Aufgabe
Extrahiere die wichtigsten Schlüsselwörter/Phrasen eines gegebenen Textinhalts.

Anforderungen
- Fasse den Textinhalt zusammen und gib die Top 5 wichtigen Schlüsselwörter/Phrasen an.
- Die Schlüsselwörter MÜSSEN in derselben Sprache wie der gegebene Textinhalt sein.
- Die Schlüsselwörter werden durch ENGLISCHES KOMMA getrennt.
- Gib NUR Schlüsselwörter aus.`,
          questions: `Rolle
Du bist ein Textanalysator.

Aufgabe
Schlage 3 Fragen zu einem gegebenen Textinhalt vor.

Anforderungen
- Verstehe und fasse den Textinhalt zusammen und schlage die Top 3 wichtigen Fragen vor.
- Die Fragen SOLLTEN KEINE überlappenden Bedeutungen haben.
- Die Fragen SOLLTEN den Hauptinhalt des Textes so weit wie möglich abdecken.
- Die Fragen MÜSSEN in derselben Sprache wie der gegebene Textinhalt sein.
- Eine Frage pro Zeile.
- Gib NUR Fragen aus.`,
          summary: `Handle als präziser Zusammenfasser. Deine Aufgabe ist es, eine Zusammenfassung des bereitgestellten Inhalts zu erstellen, die sowohl prägnant als auch originalgetreu ist.

Wichtige Anweisungen:
1. Genauigkeit: Stütze die Zusammenfassung strikt auf die gegebenen Informationen. Führe keine neuen Fakten, Schlussfolgerungen oder Interpretationen ein, die nicht explizit angegeben sind.
2. Sprache: Schreibe die Zusammenfassung in derselben Sprache wie der Quelltext.
3. Objektivität: Präsentiere die wichtigsten Punkte ohne Voreingenommenheit und bewahre die ursprüngliche Absicht und den Ton des Inhalts. Kommentiere nicht.
4. Prägnanz: Konzentriere dich auf die wichtigsten Ideen und lasse unwichtige Details und Füllwörter weg.`,
          metadata: `Extrahiere wichtige strukturierte Informationen aus dem gegebenen Inhalt. Gib NUR einen gültigen JSON-String ohne zusätzlichen Text aus. Wenn keine wichtigen strukturierten Informationen gefunden werden, gib ein leeres JSON-Objekt aus: {}.

Wichtige strukturierte Informationen können sein: Namen, Daten, Orte, Ereignisse, wichtige Fakten, numerische Daten oder andere extrahierbare Entitäten.`,
          toc: '',
        },
        user: {
          keywords: `Textinhalt
[Text hier einfügen]`,
          questions: `Textinhalt
[Text hier einfügen]`,
          summary: `Text zum Zusammenfassen:
[Text hier einfügen]`,
          metadata: `Inhalt: [INHALT HIER EINFÜGEN]`,
          toc: '[Text hier einfügen]',
        },
      },
      cancel: 'Abbrechen',
      swicthPromptMessage:
        'Das Prompt-Wort wird sich ändern. Bitte bestätigen Sie, ob Sie das bestehende Prompt-Wort verwerfen möchten?',
      tokenizerSearchMethodOptions: {
        full_text: 'Volltext',
        embedding: 'Embedding',
      },
      filenameEmbeddingWeight: 'Dateinamen-Embedding-Gewicht',
      tokenizerFieldsOptions: {
        text: 'Verarbeiteter Text',
        keywords: 'Schlüsselwörter',
        questions: 'Fragen',
        summary: 'Erweiterter Kontext',
      },
      imageParseMethodOptions: {
        ocr: 'OCR',
      },
      structuredOutput: {
        configuration: 'Konfiguration',
        structuredOutput: 'Strukturierte Ausgabe',
      },
      operations: 'Operationen',
      operationsOptions: {
        selectKeys: 'Schlüssel auswählen',
        literalEval: 'Literal eval',
        combine: 'Kombinieren',
        filterValues: 'Werte filtern',
        appendOrUpdate: 'Anhängen oder aktualisieren',
        removeKeys: 'Schlüssel entfernen',
        renameKeys: 'Schlüssel umbenennen',
      },
      ListOperationsOptions: {
        topN: 'Top N',
        head: 'Kopf',
        tail: 'Ende',
        sort: 'Sortieren',
        filter: 'Filtern',
        dropDuplicates: 'Duplikate entfernen',
      },
      sortMethod: 'Sortiermethode',
      SortMethodOptions: {
        asc: 'Aufsteigend',
        desc: 'Absteigend',
      },
      variableAssignerLogicalOperatorOptions: {
        overwrite: 'Überschrieben von',
        clear: 'Leeren',
        set: 'Setzen',
        add: 'Addieren',
        subtract: 'Subtrahieren',
        multiply: 'Multiplizieren',
        divide: 'Dividieren',
        append: 'Anhängen',
        extend: 'Erweitern',
        removeFirst: 'Erstes entfernen',
        removeLast: 'Letztes entfernen',
      },
      webhook: {
        name: 'Webhook',
        methods: 'Methoden',
        contentTypes: 'Inhaltstypen',
        security: 'Sicherheit',
        schema: 'Schema',
        response: 'Antwort',
        executionMode: 'Ausführungsmodus',
        executionModeTip:
          'Akzeptierte Antwort: Das System gibt sofort nach der Validierung der Anfrage eine Bestätigung zurück, während der Workflow asynchron im Hintergrund weiter ausgeführt wird. /Endgültige Antwort: Das System gibt erst nach Abschluss der Workflow-Ausführung eine Antwort zurück.',
        authMethods: 'Authentifizierungsmethoden',
        authType: 'Authentifizierungstyp',
        limit: 'Anfragelimit',
        per: 'Zeitraum',
        maxBodySize: 'Maximale Body-Größe',
        ipWhitelist: 'IP-Whitelist',
        tokenHeader: 'Token-Header',
        tokenValue: 'Token-Wert',
        username: 'Benutzername',
        password: 'Passwort',
        algorithm: 'Algorithmus',
        secret: 'Geheimnis',
        issuer: 'Aussteller',
        audience: 'Publikum',
        requiredClaims: 'Erforderliche Ansprüche',
        header: 'Header',
        status: 'Status',
        headersTemplate: 'Header-Vorlage',
        bodyTemplate: 'Body-Vorlage',
        basic: 'Basic',
        bearer: 'Bearer',
        apiKey: 'Api Key',
        queryParameters: 'Abfrageparameter',
        headerParameters: 'Header-Parameter',
        requestBodyParameters: 'Anfrage-Body-Parameter',
        streaming: 'Akzeptierte Antwort',
        immediately: 'Endgültige Antwort',
        overview: 'Übersicht',
        logs: 'Logs',
        agentStatus: 'Agentenstatus:',
      },
      saveToMemory: 'Im Gedächtnis speichern',
      retrievalFrom: 'Abruf von',
      tocDataSource: 'Datenquelle',
    },
    llmTools: {
      bad_calculator: {
        name: 'Taschenrechner',
        description:
          'Ein Werkzeug zur Berechnung der Summe zweier Zahlen (gibt falsche Antwort)',
        params: {
          a: 'Die erste Zahl',
          b: 'Die zweite Zahl',
        },
      },
    },
    modal: {
      okText: 'Bestätigen',
      cancelText: 'Abbrechen',
    },
    mcp: {
      export: 'Exportieren',
      import: 'Importieren',
      url: 'URL',
      serverType: 'Servertyp',
      addMCP: 'MCP hinzufügen',
      editMCP: 'MCP bearbeiten',
      toolsAvailable: 'Tools verfügbar',
      mcpServers: 'MCP-Server',
      mcpServer: 'MCP-Server',
      customizeTheListOfMcpServers: 'Liste der MCP-Server anpassen',
      cachedTools: 'zwischengespeicherte Tools',
      bulkManage: 'Sammelbearbeitung',
      exitBulkManage: 'Sammelbearbeitung beenden',
      selected: 'Ausgewählt',
    },
    search: {
      searchApps: 'Such-Apps',
      createSearch: 'Suche erstellen',
      searchGreeting: 'Wie kann ich Ihnen heute helfen?',
      profile: 'Profil ausblenden',
      locale: 'Gebietsschema',
      embedCode: 'Einbettungscode',
      id: 'ID',
      copySuccess: 'Kopieren erfolgreich',
      welcomeBack: 'Willkommen zurück',
      searchSettings: 'Sucheinstellungen',
      name: 'Name',
      avatar: 'Avatar',
      description: 'Beschreibung',
      datasets: 'Datensätze',
      rerankModel: 'Rerank-Modell',
      AISummary: 'KI-Zusammenfassung',
      enableWebSearch: 'Websuche aktivieren',
      enableRelatedSearch: 'Verwandte Suche aktivieren',
      showQueryMindmap: 'Abfrage-Mindmap anzeigen',
      embedApp: 'App einbetten',
      relatedSearch: 'Verwandte Suche',
      descriptionValue: 'Sie sind ein intelligenter Assistent.',
      okText: 'Speichern',
      cancelText: 'Abbrechen',
      chooseDataset: 'Bitte wählen Sie zuerst einen Datensatz aus',
    },
    language: {
      english: 'Englisch',
      chinese: 'Chinesisch',
      spanish: 'Spanisch',
      french: 'Französisch',
      german: 'Deutsch',
      japanese: 'Japanisch',
      korean: 'Koreanisch',
      vietnamese: 'Vietnamesisch',
      russian: 'Russisch',
      bulgarian: 'Bulgarisch',
      arabic: 'Arabisch',
    },
    pagination: {
      total: 'Gesamt {{total}}',
      page: '{{page}} / Seite',
    },
    dataflowParser: {
      result: 'Ergebnis',
      parseSummary: 'Analyse-Zusammenfassung',
      parseSummaryTip: 'Parser：DeepDoc',
      parserMethod: 'Parser-Methode',
      outputFormat: 'Ausgabeformat',
      rerunFromCurrentStep: 'Vom aktuellen Schritt erneut ausführen',
      rerunFromCurrentStepTip:
        'Änderungen erkannt. Klicken Sie zum erneuten Ausführen.',
      confirmRerun: 'Erneuten Ausführungsprozess bestätigen',
      confirmRerunModalContent: `
      <p class="text-sm text-text-disabled font-medium mb-2">
        Sie sind dabei, den Prozess ab dem Schritt <span class="text-text-secondary">{{step}}</span> erneut auszuführen.
      </p>
      <p class="text-sm mb-3 text-text-disabled">Dies wird:</p><br />
      <ul class="list-disc list-inside space-y-1 text-sm text-text-secondary">
        <li>• Vorhandene Ergebnisse ab dem aktuellen Schritt überschreiben</li>
        <li>• Einen neuen Log-Eintrag zur Nachverfolgung erstellen</li>
        <li>• Vorherige Schritte bleiben unverändert</li>
      </ul>`,
      changeStepModalTitle: 'Schrittwechsel-Warnung',
      changeStepModalContent: `
      <p>Sie bearbeiten derzeit die Ergebnisse dieser Stufe.</p>
      <p>Wenn Sie zu einer späteren Stufe wechseln, gehen Ihre Änderungen verloren. </p>
      <p>Um sie zu behalten, klicken Sie bitte auf Erneut ausführen, um die aktuelle Stufe erneut auszuführen.</p> `,
      changeStepModalConfirmText: 'Trotzdem wechseln',
      changeStepModalCancelText: 'Abbrechen',
      unlinkPipelineModalTitle: 'Dateneingabe-Pipeline trennen',
      unlinkPipelineModalConfirmText: 'Trennen',
      unlinkPipelineModalContent: `
      <p>Nach dem Trennen ist dieser Datensatz nicht mehr mit der aktuellen Dateneingabe-Pipeline verbunden.</p>
      <p>Dateien, die bereits analysiert werden, werden bis zum Abschluss fortgesetzt</p>
      <p>Dateien, die noch nicht analysiert wurden, werden nicht mehr verarbeitet</p> <br/>
      <p>Sind Sie sicher, dass Sie fortfahren möchten?</p> `,
      unlinkSourceModalTitle: 'Datenquelle trennen',
      unlinkSourceModalContent: `
      <p>Sind Sie sicher, dass Sie diese Datenquelle trennen möchten?</p>`,
      unlinkSourceModalConfirmText: 'Trennen',
    },
    datasetOverview: {
      downloadTip: 'Dateien werden von Datenquellen heruntergeladen. ',
      processingTip:
        'Dateien werden von der Dateneingabe-Pipeline verarbeitet.',
      totalFiles: 'Gesamtdateien',
      downloading: 'Wird heruntergeladen',
      downloadSuccessTip: 'Gesamte erfolgreiche Downloads',
      downloadFailedTip: 'Gesamte fehlgeschlagene Downloads',
      processingSuccessTip: 'Gesamte erfolgreich verarbeitete Dateien',
      processingFailedTip: 'Gesamte fehlgeschlagene Prozesse',
      processing: 'Verarbeitung',
      noData: 'Noch kein Log',
    },
    deleteModal: {
      delAgent: 'Agent löschen',
      delDataset: 'Datensatz löschen',
      delSearch: 'Suche löschen',
      delFile: 'Datei löschen',
      delFiles: 'Dateien löschen',
      delFilesContent: '{{count}} Dateien ausgewählt',
      delChat: 'Chat löschen',
      delMember: 'Mitglied löschen',
      delMemory: 'Gedächtnis löschen',
    },
    empty: {
      noMCP: 'Keine MCP-Server verfügbar',
      agentTitle: 'Noch keine Agenten-App erstellt',
      notFoundAgent: 'Agenten-App nicht gefunden',
      datasetTitle: 'Noch kein Datensatz erstellt',
      notFoundDataset: 'Datensatz nicht gefunden',
      chatTitle: 'Noch keine Chat-App erstellt',
      notFoundChat: 'Chat-App nicht gefunden',
      searchTitle: 'Noch keine Such-App erstellt',
      notFoundSearch: 'Such-App nicht gefunden',
      memoryTitle: 'Noch kein Gedächtnis erstellt',
      notFoundMemory: 'Gedächtnis nicht gefunden',
      addNow: 'Jetzt hinzufügen',
    },
    admin: {
      loginTitle: 'Admin-Konsole',
      title: 'RAGFlow',
      confirm: 'Bestätigen',
      close: 'Schließen',
      yes: 'Ja',
      no: 'Nein',
      delete: 'Löschen',
      cancel: 'Abbrechen',
      reset: 'Zurücksetzen',
      import: 'Importieren',
      description: 'Beschreibung',
      noDescription: 'Keine Beschreibung',
      resourceType: {
        dataset: 'Datensatz',
        chat: 'Chat',
        agent: 'Agent',
        search: 'Suche',
        file: 'Datei',
        team: 'Team',
        memory: 'Gedächtnis',
      },
      permissionType: {
        enable: 'Aktivieren',
        read: 'Lesen',
        write: 'Schreiben',
        share: 'Teilen',
      },
      serviceStatus: 'Dienststatus',
      userManagement: 'Benutzerverwaltung',
      registrationWhitelist: 'Registrierungs-Whitelist',
      roles: 'Rollen',
      monitoring: 'Überwachung',
      back: 'Zurück',
      active: 'Aktiv',
      inactive: 'Inaktiv',
      enable: 'Aktivieren',
      disable: 'Deaktivieren',
      all: 'Alle',
      actions: 'Aktionen',
      newUser: 'Neuer Benutzer',
      email: 'E-Mail',
      name: 'Name',
      nickname: 'Spitzname',
      status: 'Status',
      id: 'ID',
      serviceType: 'Diensttyp',
      host: 'Host',
      port: 'Port',
      role: 'Rolle',
      user: 'Benutzer',
      superuser: 'Superuser',
      createTime: 'Erstellungszeit',
      lastLoginTime: 'Letzte Anmeldezeit',
      lastUpdateTime: 'Letzte Aktualisierungszeit',
      isAnonymous: 'Ist anonym',
      isSuperuser: 'Ist Superuser',
      deleteUser: 'Benutzer löschen',
      deleteUserConfirmation:
        'Sind Sie sicher, dass Sie diesen Benutzer löschen möchten?',
      createNewUser: 'Neuen Benutzer erstellen',
      changePassword: 'Passwort ändern',
      newPassword: 'Neues Passwort',
      confirmNewPassword: 'Neues Passwort bestätigen',
      password: 'Passwort',
      confirmPassword: 'Passwort bestätigen',
      invalidEmail: 'Bitte geben Sie eine gültige E-Mail-Adresse ein!',
      passwordRequired: 'Bitte geben Sie Ihr Passwort ein!',
      passwordMinLength: 'Passwort muss mehr als 8 Zeichen haben.',
      confirmPasswordRequired: 'Bitte bestätigen Sie Ihr Passwort!',
      confirmPasswordDoNotMatch:
        'Die eingegebenen Passwörter stimmen nicht überein!',
      read: 'Lesen',
      write: 'Schreiben',
      share: 'Teilen',
      create: 'Erstellen',
      extraInfo: 'Zusatzinformationen',
      serviceDetail: 'Dienst {{name}} Detail',
      taskExecutorDetail: 'Aufgabenausführer Detail',
      whitelistManagement: 'Whitelist-Verwaltung',
      exportAsExcel: 'Excel exportieren',
      importFromExcel: 'Excel importieren',
      createEmail: 'E-Mail erstellen',
      deleteEmail: 'E-Mail löschen',
      editEmail: 'E-Mail bearbeiten',
      deleteWhitelistEmailConfirmation:
        'Sind Sie sicher, dass Sie diese E-Mail aus der Whitelist löschen möchten? Diese Aktion kann nicht rückgängig gemacht werden.',
      importWhitelist: 'Whitelist importieren (Excel)',
      importSelectExcelFile: 'Excel-Datei (.xlsx)',
      importOverwriteExistingEmails: 'Vorhandene E-Mails überschreiben',
      importInvalidExcelFile: 'Bitte wählen Sie eine gültige Excel-Datei',
      importFileRequired: 'Bitte wählen Sie eine Datei zum Importieren',
      importFileTips:
        'Datei muss eine einzelne Kopfzeilenspalte namens <code>email</code> enthalten.',
      chunkNum: 'Chunks',
      docNum: 'Dokumente',
      tokenNum: 'Verwendete Tokens',
      language: 'Sprache',
      createDate: 'Erstellungsdatum',
      updateDate: 'Aktualisierungsdatum',
      permission: 'Berechtigung',
      agentTitle: 'Agententitel',
      canvasCategory: 'Canvas-Kategorie',
      newRole: 'Neue Rolle',
      addNewRole: 'Neue Rolle hinzufügen',
      roleName: 'Rollenname',
      roleNameRequired: 'Rollenname ist erforderlich',
      resources: 'Ressourcen',
      editRoleDescription: 'Rollenbeschreibung bearbeiten',
      deleteRole: 'Rolle löschen',
      deleteRoleConfirmation:
        'Sind Sie sicher, dass Sie diese Rolle löschen möchten? Diese Aktion kann nicht rückgängig gemacht werden.',
      alive: 'Aktiv',
      timeout: 'Zeitüberschreitung',
      fail: 'Fehlgeschlagen',
    },
  },
};
