import type { Translation } from '../translation-keys.ts';

export const tr: Translation = {
  collapse: 'Daralt',
  expand: 'Genişlet',

  fieldDescriptionPlaceholder: 'Bu alanın amacını açıklayın',
  fieldDelete: 'Alanı sil',
  fieldDescription: 'Açıklama',
  fieldDescriptionTooltip: 'Bu alanın ne temsil ettiğine dair bağlam ekleyin',
  fieldNameLabel: 'Alan Adı',
  fieldNamePlaceholder: 'örn. adSoyad, yaş, aktifMi',
  fieldNameTooltip:
    'Daha iyi okunabilirlik için camelCase kullanın (örn. adSoyad)',
  fieldRequiredLabel: 'Zorunlu Alan',
  fieldType: 'Alan Türü',
  fieldTypeExample: 'Örnek:',
  fieldTypeTooltipString: 'string: Metin',
  fieldTypeTooltipNumber: 'number: Sayısal',
  fieldTypeTooltipBoolean: 'boolean: Doğru/yanlış',
  fieldTypeTooltipObject: 'object: İç içe JSON',
  fieldTypeTooltipArray: 'array: Değer listeleri',
  fieldAddNewButton: 'Alan Ekle',
  fieldAddNewBadge: 'Şema Oluşturucu',
  fieldAddNewCancel: 'İptal',
  fieldAddNewConfirm: 'Alan Ekle',
  fieldAddNewDescription: 'JSON şemanız için yeni bir alan oluşturun',
  fieldAddNewLabel: 'Yeni Alan Ekle',

  fieldTypeTextLabel: 'Metin',
  fieldTypeTextDescription: 'Adlar, açıklamalar vb. metin değerleri için',
  fieldTypeNumberLabel: 'Sayı',
  fieldTypeNumberDescription: 'Ondalıklı veya tam sayılar için',
  fieldTypeBooleanLabel: 'Evet/Hayır',
  fieldTypeBooleanDescription: 'Doğru/yanlış değerleri için',
  fieldTypeObjectLabel: 'Grup',
  fieldTypeObjectDescription: 'İlgili alanları bir araya getirmek için',
  fieldTypeArrayLabel: 'Liste',
  fieldTypeArrayDescription: 'Öğe koleksiyonları için',

  propertyDescriptionPlaceholder: 'Açıklama ekle...',
  propertyDescriptionButton: 'Açıklama ekle...',
  propertyRequired: 'Zorunlu',
  propertyOptional: 'İsteğe bağlı',
  propertyDelete: 'Alanı sil',

  schemaEditorTitle: 'JSON Şema Düzenleyici',
  schemaEditorToggleFullscreen: 'Tam ekranı aç/kapat',
  schemaEditorEditModeVisual: 'Görsel',
  schemaEditorEditModeJson: 'JSON',

  arrayMinimumLabel: 'Minimum Öğe',
  arrayMinimumPlaceholder: 'Minimum yok',
  arrayMaximumLabel: 'Maksimum Öğe',
  arrayMaximumPlaceholder: 'Maksimum yok',
  arrayForceUniqueItemsLabel: 'Benzersiz öğeleri zorla',
  arrayItemTypeLabel: 'Öğe Türü',
  arrayValidationErrorMinMax: "'minItems' 'maxItems'ten büyük olamaz.",
  arrayValidationErrorContainsMinMax:
    "'minContains' 'maxContains'ten büyük olamaz.",

  booleanAllowFalseLabel: 'Yanlış değere izin ver',
  booleanAllowTrueLabel: 'Doğru değere izin ver',
  booleanNeitherWarning: 'Uyarı: En az bir değere izin vermelisiniz.',

  numberMinimumLabel: 'Minimum Değer',
  numberMinimumPlaceholder: 'Minimum yok',
  numberMaximumLabel: 'Maksimum Değer',
  numberMaximumPlaceholder: 'Maksimum yok',
  numberExclusiveMinimumLabel: 'Özel Minimum',
  numberExclusiveMinimumPlaceholder: 'Özel minimum yok',
  numberExclusiveMaximumLabel: 'Özel Maksimum',
  numberExclusiveMaximumPlaceholder: 'Özel maksimum yok',
  numberMultipleOfLabel: 'Katı',
  numberMultipleOfPlaceholder: 'Herhangi',
  numberAllowedValuesEnumLabel: 'İzin Verilen Değerler (enum)',
  numberAllowedValuesEnumNone: 'Kısıtlı değer ayarlanmadı',
  numberAllowedValuesEnumAddLabel: 'Ekle',
  numberAllowedValuesEnumAddPlaceholder: 'İzin verilen değer ekle...',
  numberValidationErrorMinMax:
    'Minimum ve maksimum değerler tutarlı olmalıdır.',
  numberValidationErrorBothExclusiveAndInclusiveMin:
    "'exclusiveMinimum' ve 'minimum' aynı anda ayarlanamaz.",
  numberValidationErrorBothExclusiveAndInclusiveMax:
    "'exclusiveMaximum' ve 'maximum' aynı anda ayarlanamaz.",
  numberValidationErrorEnumOutOfRange:
    'Enum değerleri tanımlı aralık içinde olmalıdır.',

  objectPropertiesNone: 'Tanımlı özellik yok',
  objectValidationErrorMinMax:
    "'minProperties' 'maxProperties'ten büyük olamaz.",

  stringMinimumLengthLabel: 'Minimum Uzunluk',
  stringMinimumLengthPlaceholder: 'Minimum yok',
  stringMaximumLengthLabel: 'Maksimum Uzunluk',
  stringMaximumLengthPlaceholder: 'Maksimum yok',
  stringPatternLabel: 'Desen (regex)',
  stringPatternPlaceholder: '^[a-zA-Z]+$',
  stringFormatLabel: 'Format',
  stringFormatNone: 'Yok',
  stringFormatDateTime: 'Tarih-Saat',
  stringFormatDate: 'Tarih',
  stringFormatTime: 'Saat',
  stringFormatEmail: 'E-posta',
  stringFormatUri: 'URI',
  stringFormatUuid: 'UUID',
  stringFormatHostname: 'Ana Bilgisayar Adı',
  stringFormatIpv4: 'IPv4 Adresi',
  stringFormatIpv6: 'IPv6 Adresi',
  stringAllowedValuesEnumLabel: 'İzin Verilen Değerler (enum)',
  stringAllowedValuesEnumNone: 'Kısıtlı değer ayarlanmadı',
  stringAllowedValuesEnumAddPlaceholder: 'İzin verilen değer ekle...',
  stringValidationErrorLengthRange:
    "'Minimum Uzunluk' 'Maksimum Uzunluk'tan büyük olamaz.",

  schemaTypeArray: 'Liste',
  schemaTypeBoolean: 'Evet/Hayır',
  schemaTypeNumber: 'Sayı',
  schemaTypeObject: 'Nesne',
  schemaTypeString: 'Metin',
  schemaTypeNull: 'Boş',

  inferrerTitle: 'JSON Şeması Çıkar',
  inferrerDescription:
    'Şema oluşturmak için JSON belgenizi aşağıya yapıştırın.',
  inferrerCancel: 'İptal',
  inferrerGenerate: 'Şema Oluştur',
  inferrerErrorInvalidJson:
    'Geçersiz JSON formatı. Lütfen girdinizi kontrol edin.',

  validatorTitle: 'JSON Doğrula',
  validatorDescription:
    'JSON belgenizi geçerli şemaya göre doğrulamak için yapıştırın. Doğrulama siz yazarken otomatik olarak gerçekleşir.',
  validatorCurrentSchema: 'Geçerli Şema:',
  validatorContent: "JSON'unuz:",
  validatorValid: 'JSON şemaya göre geçerli!',
  validatorErrorInvalidSyntax: 'Geçersiz JSON sözdizimi',
  validatorErrorSchemaValidation: 'Şema doğrulama hatası',
  validatorErrorCount: '{count} doğrulama hatası tespit edildi',
  validatorErrorPathRoot: 'Kök',
  validatorErrorLocationLineAndColumn: 'Satır {line}, Sütun {column}',
  validatorErrorLocationLineOnly: 'Satır {line}',

  visualizerDownloadTitle: 'Şemayı İndir',
  visualizerDownloadFileName: 'schema.json',
  visualizerSource: 'JSON Şema Kaynağı',

  visualEditorNoFieldsHint1: 'Henüz alan tanımlanmadı',
  visualEditorNoFieldsHint2: 'Başlamak için ilk alanınızı ekleyin',

  typeValidationErrorNegativeLength: 'Uzunluk değerleri negatif olamaz.',
  typeValidationErrorIntValue: 'Değer bir tam sayı olmalıdır.',
  typeValidationErrorPositive: 'Değer pozitif olmalıdır.',
};
