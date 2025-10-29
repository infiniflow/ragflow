import type { Translation } from '../translation-keys.ts';

export const ru: Translation = {
  collapse: 'Свернуть',
  expand: 'Развернуть',

  fieldDescriptionPlaceholder: 'Опишите назначение этого поля',
  fieldDelete: 'Удалить поле',
  fieldDescription: 'Описание',
  fieldDescriptionTooltip: 'Добавьте контекст о том, что представляет это поле',
  fieldNameLabel: 'Имя поля',
  fieldNamePlaceholder: 'например, имя, возраст, активен',
  fieldNameTooltip:
    'Используйте camelCase для лучшей читаемости (например, firstName)',
  fieldRequiredLabel: 'Обязательное поле',
  fieldType: 'Тип поля',
  fieldTypeExample: 'Пример:',
  fieldTypeTooltipString: 'строка: Текст',
  fieldTypeTooltipNumber: 'число: Числовое значение',
  fieldTypeTooltipBoolean: 'логическое: Истина/ложь',
  fieldTypeTooltipObject: 'объект: Вложенный JSON',
  fieldTypeTooltipArray: 'массив: Списки значений',
  fieldAddNewButton: 'Добавить поле',
  fieldAddNewBadge: 'Конструктор схем',
  fieldAddNewCancel: 'Отмена',
  fieldAddNewConfirm: 'Добавить поле',
  fieldAddNewDescription: 'Создайте новое поле для вашей схемы JSON',
  fieldAddNewLabel: 'Добавить новое поле',

  fieldTypeTextLabel: 'Текст',
  fieldTypeTextDescription:
    'Для текстовых значений, таких как имена, описания и т.д.',
  fieldTypeNumberLabel: 'Число',
  fieldTypeNumberDescription: 'Для десятичных или целых чисел',
  fieldTypeBooleanLabel: 'Да/Нет',
  fieldTypeBooleanDescription: 'Для значений истина/ложь',
  fieldTypeObjectLabel: 'Группа',
  fieldTypeObjectDescription: 'Для группировки связанных полей вместе',
  fieldTypeArrayLabel: 'Список',
  fieldTypeArrayDescription: 'Для коллекций элементов',

  propertyDescriptionPlaceholder: 'Добавить описание...',
  propertyDescriptionButton: 'Добавить описание...',
  propertyRequired: 'Обязательное',
  propertyOptional: 'Необязательное',
  propertyDelete: 'Удалить поле',

  schemaEditorTitle: 'Редактор JSON схем',
  schemaEditorToggleFullscreen: 'Переключить полноэкранный режим',
  schemaEditorEditModeVisual: 'Визуальный',
  schemaEditorEditModeJson: 'JSON',

  arrayMinimumLabel: 'Минимум элементов',
  arrayMinimumPlaceholder: 'Нет минимума',
  arrayMaximumLabel: 'Максимум элементов',
  arrayMaximumPlaceholder: 'Нет максимума',
  arrayForceUniqueItemsLabel: 'Требовать уникальные элементы',
  arrayItemTypeLabel: 'Тип элемента',
  arrayValidationErrorMinMax: "'minItems' не может быть больше 'maxItems'.",
  arrayValidationErrorContainsMinMax:
    "'minContains' не может быть больше 'maxContains'.",

  booleanAllowFalseLabel: 'Разрешить значение ложь',
  booleanAllowTrueLabel: 'Разрешить значение истина',
  booleanNeitherWarning: 'Внимание: Вы должны разрешить хотя бы одно значение.',

  numberMinimumLabel: 'Минимальное значение',
  numberMinimumPlaceholder: 'Нет минимума',
  numberMaximumLabel: 'Максимальное значение',
  numberMaximumPlaceholder: 'Нет максимума',
  numberExclusiveMinimumLabel: 'Исключающее минимальное',
  numberExclusiveMinimumPlaceholder: 'Нет исключающего минимума',
  numberExclusiveMaximumLabel: 'Исключающее максимальное',
  numberExclusiveMaximumPlaceholder: 'Нет исключающего максимума',
  numberMultipleOfLabel: 'Кратно',
  numberMultipleOfPlaceholder: 'Любое',
  numberAllowedValuesEnumLabel: 'Разрешенные значения (enum)',
  numberAllowedValuesEnumNone: 'Нет ограниченных значений',
  numberAllowedValuesEnumAddLabel: 'Добавить',
  numberAllowedValuesEnumAddPlaceholder: 'Добавить разрешенное значение...',
  numberValidationErrorMinMax:
    'Минимальное и максимальное значения должны быть согласованы.',
  numberValidationErrorBothExclusiveAndInclusiveMin:
    "Оба поля 'exclusiveMinimum' и 'minimum' не могут быть установлены одновременно.",
  numberValidationErrorBothExclusiveAndInclusiveMax:
    "Оба поля 'exclusiveMaximum' и 'maximum' не могут быть установлены одновременно.",
  numberValidationErrorEnumOutOfRange:
    'Значения перечисления должны быть в пределах определенного диапазона.',

  objectPropertiesNone: 'Нет определенных свойств',
  objectValidationErrorMinMax:
    "'minProperties' не может быть больше 'maxProperties'.",

  stringMinimumLengthLabel: 'Минимальная длина',
  stringMinimumLengthPlaceholder: 'Нет минимума',
  stringMaximumLengthLabel: 'Максимальная длина',
  stringMaximumLengthPlaceholder: 'Нет максимума',
  stringPatternLabel: 'Шаблон (regex)',
  stringPatternPlaceholder: '^[a-zA-Z]+$',
  stringFormatLabel: 'Формат',
  stringFormatNone: 'Нет',
  stringFormatDateTime: 'Дата-Время',
  stringFormatDate: 'Дата',
  stringFormatTime: 'Время',
  stringFormatEmail: 'Email',
  stringFormatUri: 'URI',
  stringFormatUuid: 'UUID',
  stringFormatHostname: 'Имя хоста',
  stringFormatIpv4: 'Адрес IPv4',
  stringFormatIpv6: 'Адрес IPv6',
  stringAllowedValuesEnumLabel: 'Разрешенные значения (enum)',
  stringAllowedValuesEnumNone: 'Нет ограниченных значений',
  stringAllowedValuesEnumAddPlaceholder: 'Добавить разрешенное значение...',
  stringValidationErrorLengthRange:
    "'Минимальная длина' не может быть больше 'Максимальной длины'.",

  schemaTypeArray: 'Список',
  schemaTypeBoolean: 'Да/Нет',
  schemaTypeNumber: 'Число',
  schemaTypeObject: 'Объект',
  schemaTypeString: 'Текст',
  schemaTypeNull: 'Пусто',

  inferrerTitle: 'Вывести схему JSON',
  inferrerDescription:
    'Вставьте ваш документ JSON ниже, чтобы сгенерировать из него схему.',
  inferrerCancel: 'Отмена',
  inferrerGenerate: 'Сгенерировать схему',
  inferrerErrorInvalidJson:
    'Неверный формат JSON. Пожалуйста, проверьте ваши данные.',

  validatorTitle: 'Проверить JSON',
  validatorDescription:
    'Вставьте ваш документ JSON для проверки по текущей схеме. Проверка происходит автоматически по мере ввода.',
  validatorCurrentSchema: 'Текущая схема:',
  validatorContent: 'Ваш JSON:',
  validatorValid: 'JSON действителен в соответствии со схемой!',
  validatorErrorInvalidSyntax: 'Неверный синтаксис JSON',
  validatorErrorSchemaValidation: 'Ошибка проверки схемы',
  validatorErrorCount: 'Обнаружено ошибок проверки: {count}',
  validatorErrorPathRoot: 'Корень',
  validatorErrorLocationLineAndColumn: 'Строка {line}, столбец {column}',
  validatorErrorLocationLineOnly: 'Строка {line}',

  visualizerDownloadTitle: 'Скачать схему',
  visualizerDownloadFileName: 'schema.json',
  visualizerSource: 'Источник схемы JSON',

  visualEditorNoFieldsHint1: 'Пока не определено ни одного поля',
  visualEditorNoFieldsHint2: 'Добавьте ваше первое поле, чтобы начать',

  typeValidationErrorNegativeLength:
    'Значения длины не могут быть отрицательными.',
  typeValidationErrorIntValue: 'Значение должно быть целым числом.',
  typeValidationErrorPositive: 'Значение должно быть положительным.',
};
