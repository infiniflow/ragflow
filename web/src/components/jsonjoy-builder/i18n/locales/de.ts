import type { Translation } from '../translation-keys.ts';

export const de: Translation = {
  collapse: 'Einklappen',
  expand: 'Ausklappen',

  fieldDescriptionPlaceholder: 'Zweck dieses Felds beschreiben',
  fieldDelete: 'Feld löschen',
  fieldDescription: 'Beschreibung',
  fieldDescriptionTooltip: 'Kontext zur Bedeutung dieses Felds hinzufügen',
  fieldNameLabel: 'Feldname',
  fieldNamePlaceholder: 'z.B. firstName, age, isActive',
  fieldNameTooltip:
    'CamelCase für bessere Lesbarkeit verwenden (z.B. firstName)',
  fieldRequiredLabel: 'Pflichtfeld',
  fieldType: 'Feldart',
  fieldTypeExample: 'Beispiel:',
  fieldTypeTooltipString: 'string: Text',
  fieldTypeTooltipNumber: 'number: Zahl',
  fieldTypeTooltipBoolean: 'boolean: Wahr/Falsch',
  fieldTypeTooltipObject: 'object: Verschachteltes JSON',
  fieldTypeTooltipArray: 'array: Liste von Werten',
  fieldAddNewButton: 'Feld hinzufügen',
  fieldAddNewBadge: 'Schema-Builder',
  fieldAddNewCancel: 'Abbrechen',
  fieldAddNewConfirm: 'Feld hinzufügen',
  fieldAddNewDescription: 'Neues Feld für das JSON-Schema erstellen',
  fieldAddNewLabel: 'Neues Feld hinzufügen',

  fieldTypeTextLabel: 'Text',
  fieldTypeTextDescription: 'Für Textwerte wie Namen, Beschreibungen usw.',
  fieldTypeNumberLabel: 'Zahl',
  fieldTypeNumberDescription: 'Für Dezimal- oder Ganzzahlen',
  fieldTypeBooleanLabel: 'Ja/Nein',
  fieldTypeBooleanDescription: 'Für Wahr/Falsch-Werte',
  fieldTypeObjectLabel: 'Gruppe',
  fieldTypeObjectDescription: 'Zum Gruppieren verwandter Felder',
  fieldTypeArrayLabel: 'Liste',
  fieldTypeArrayDescription: 'Für Sammlungen von Elementen',

  propertyDescriptionPlaceholder: 'Beschreibung hinzufügen...',
  propertyDescriptionButton: 'Beschreibung hinzufügen...',
  propertyRequired: 'Erforderlich',
  propertyOptional: 'Optional',
  propertyDelete: 'Feld löschen',

  schemaEditorTitle: 'JSON-Schema-Editor',
  schemaEditorToggleFullscreen: 'Vollbild umschalten',
  schemaEditorEditModeVisual: 'Visuell',
  schemaEditorEditModeJson: 'JSON',

  arrayMinimumLabel: 'Mindestanzahl an Elementen',
  arrayMinimumPlaceholder: 'Kein Minimum',
  arrayMaximumLabel: 'Maximale Anzahl an Elemente',
  arrayMaximumPlaceholder: 'Kein Maximum',
  arrayForceUniqueItemsLabel: 'Nur eindeutige Elemente erlauben',
  arrayItemTypeLabel: 'Elementtyp',
  arrayValidationErrorMinMax:
    "'minItems' darf nicht größer als 'maxItems' sein.",
  arrayValidationErrorContainsMinMax:
    "'minContains' darf nicht größer als 'maxContains' sein.",

  booleanAllowFalseLabel: 'Falsch-Werte erlauben',
  booleanAllowTrueLabel: 'Wahr-Werte erlauben',
  booleanNeitherWarning:
    'Achtung: Mindestens einer von beiden Werten muss erlaubt sein.',

  numberMinimumLabel: 'Minimalwert',
  numberMinimumPlaceholder: 'Kein Minimum',
  numberMaximumLabel: 'Maximalwert',
  numberMaximumPlaceholder: 'Kein Maximum',
  numberExclusiveMinimumLabel: 'Exklusives Minimum',
  numberExclusiveMinimumPlaceholder: 'Kein exklusives Minimum',
  numberExclusiveMaximumLabel: 'Exklusives Maximum',
  numberExclusiveMaximumPlaceholder: 'Kein exklusives Maximum',
  numberMultipleOfLabel: 'Vielfaches von',
  numberMultipleOfPlaceholder: 'Beliebig',
  numberAllowedValuesEnumLabel: 'Erlaubte Werte (Enum)',
  numberAllowedValuesEnumNone: 'Keine Einschränkung für Werte festgelegt',
  numberAllowedValuesEnumAddLabel: 'Hinzufügen',
  numberAllowedValuesEnumAddPlaceholder: 'Erlaubten Wert hinzufügen...',
  numberValidationErrorMinMax: 'Minimum und Maximum müssen konsistent sein.',
  numberValidationErrorBothExclusiveAndInclusiveMin:
    "Sowohl 'exclusiveMinimum' als auch 'minimum' dürfen nicht gleichzeitig festgelegt werden.",
  numberValidationErrorBothExclusiveAndInclusiveMax:
    "Sowohl 'exclusiveMaximum' als auch 'maximum' dürfen nicht gleichzeitig festgelegt werden.",
  numberValidationErrorEnumOutOfRange:
    'Enum-Werte müssen innerhalb des definierten Bereichs liegen.',

  objectPropertiesNone: 'Keine Eigenschaften definiert',
  objectValidationErrorMinMax:
    "'minProperties' darf nicht größer als 'maxProperties' sein.",

  stringMinimumLengthLabel: 'Minimale Länge',
  stringMinimumLengthPlaceholder: 'Kein Minimum',
  stringMaximumLengthLabel: 'Maximale Länge',
  stringMaximumLengthPlaceholder: 'Kein Maximum',
  stringPatternLabel: 'Muster (Regex)',
  stringPatternPlaceholder: '^[a-zA-Z]+$',
  stringFormatLabel: 'Format',
  stringFormatNone: 'Keins',
  stringFormatDateTime: 'Datum und Uhrzeit',
  stringFormatDate: 'Datum',
  stringFormatTime: 'Uhrzeit',
  stringFormatEmail: 'E-Mail',
  stringFormatUri: 'URI',
  stringFormatUuid: 'UUID',
  stringFormatHostname: 'Hostname',
  stringFormatIpv4: 'IPv4-Adresse',
  stringFormatIpv6: 'IPv6-Adresse',
  stringAllowedValuesEnumLabel: 'Erlaubte Werte (Enum)',
  stringAllowedValuesEnumNone: 'Keine Einschränkung für Werte festgelegt',
  stringAllowedValuesEnumAddPlaceholder: 'Erlaubten Wert hinzufügen...',
  stringValidationErrorLengthRange:
    "'Minimale Länge' darf nicht größer als 'Maximale Länge' sein.",

  schemaTypeArray: 'Liste',
  schemaTypeBoolean: 'Ja/Nein',
  schemaTypeNumber: 'Zahl',
  schemaTypeObject: 'Objekt',
  schemaTypeString: 'Text',
  schemaTypeNull: 'Leer',

  inferrerTitle: 'JSON-Schema ableiten',
  inferrerDescription:
    'JSON-Dokument unten einfügen, um ein Schema daraus zu erstellen.',
  inferrerCancel: 'Abbrechen',
  inferrerGenerate: 'Schema erstellen',
  inferrerErrorInvalidJson: 'Ungültiges JSON-Format. Bitte Eingabe prüfen.',

  validatorTitle: 'JSON validieren',
  validatorDescription:
    'JSON-Dokument einfügen, um es gegen das aktuelle Schema zu prüfen. Die Validierung erfolgt automatisch beim Tippen.',
  validatorCurrentSchema: 'Aktuelles Schema:',
  validatorContent: 'Zu prüfendes JSON:',
  validatorValid: 'JSON ist gültig zum Schema!',
  validatorErrorInvalidSyntax: 'Ungültige JSON-Syntax',
  validatorErrorSchemaValidation: 'Schema-Validierungsfehler',
  validatorErrorCount: '{count} Validierungsfehler gefunden',
  validatorErrorPathRoot: 'Wurzel',
  validatorErrorLocationLineAndColumn: 'Zeile {line}, Spalte {column}',
  validatorErrorLocationLineOnly: 'Zeile {line}',

  visualizerDownloadTitle: 'Schema herunterladen',
  visualizerDownloadFileName: 'schema.json',
  visualizerSource: 'JSON-Schema-Quelle',

  visualEditorNoFieldsHint1: 'Noch keine Felder definiert',
  visualEditorNoFieldsHint2: 'Erstes Feld hinzufügen, um zu starten',

  typeValidationErrorNegativeLength: 'Längenwerte dürfen nicht negativ sein.',
  typeValidationErrorIntValue: 'Der Wert muss eine ganze Zahl sein.',
  typeValidationErrorPositive: 'Der Wert muss positiv sein.',
};
