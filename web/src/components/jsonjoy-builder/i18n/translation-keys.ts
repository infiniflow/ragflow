export interface Translation {
  /**
   * The translation for the key `collapse`. English default is:
   *
   * > Collapse
   */
  readonly collapse: string;
  /**
   * The translation for the key `expand`. English default is:
   *
   * > Expand
   */
  readonly expand: string;

  /**
   * The translation for the key `fieldDelete`. English default is:
   *
   * > Delete field
   */
  readonly fieldDelete: string;
  /**
   * The translation for the key `fieldDescriptionPlaceholder`. English default is:
   *
   * > Describe the purpose of this field
   */
  readonly fieldDescriptionPlaceholder: string;
  /**
   * The translation for the key `fieldNamePlaceholder`. English default is:
   *
   * > e.g. firstName, age, isActive
   */
  readonly fieldNamePlaceholder: string;
  /**
   * The translation for the key `fieldNameLabel`. English default is:
   *
   * > Field Name
   */
  readonly fieldNameLabel: string;
  /**
   * The translation for the key `fieldNameTooltip`. English default is:
   *
   * > Use camelCase for better readability (e.g., firstName)
   */
  readonly fieldNameTooltip: string;
  /**
   * The translation for the key `fieldRequiredLabel`. English default is:
   *
   * > Required Field
   */
  readonly fieldRequiredLabel: string;
  /**
   * The translation for the key `fieldDescription`. English default is:
   *
   * > Description
   */
  readonly fieldDescription: string;
  /**
   * The translation for the key `fieldDescriptionTooltip`. English default is:
   *
   * > Add context about what this field represents
   */
  readonly fieldDescriptionTooltip: string;
  /**
   * The translation for the key `fieldType`. English default is:
   *
   * > Field Type
   */
  readonly fieldType: string;
  /**
   * The translation for the key `fieldTypeExample`. English default is:
   *
   * > Example:
   */
  readonly fieldTypeExample: string;
  /**
   * The translation for the key `fieldTypeTooltipString`. English default is:
   *
   * > string: Text
   */
  readonly fieldTypeTooltipString: string;
  /**
   * The translation for the key `fieldTypeTooltipNumber`. English default is:
   *
   * > number: Numeric
   */
  readonly fieldTypeTooltipNumber: string;
  /**
   * The translation for the key `fieldTypeTooltipBoolean`. English default is:
   *
   * > boolean: True/false
   */
  readonly fieldTypeTooltipBoolean: string;
  /**
   * The translation for the key `fieldTypeTooltipObject`. English default is:
   *
   * > object: Nested JSON
   */
  readonly fieldTypeTooltipObject: string;
  /**
   * The translation for the key `fieldTypeTooltipArray`. English default is:
   *
   * > array: Lists of values
   */
  readonly fieldTypeTooltipArray: string;

  /**
   * The translation for the key `fieldAddNewButton`. English default is:
   *
   * > Add Field
   */
  readonly fieldAddNewButton: string;
  /**
   * The translation for the key `fieldAddNewLabel`. English default is:
   *
   * > Add New Field
   */
  readonly fieldAddNewLabel: string;
  /**
   * The translation for the key `fieldAddNewDescription`. English default is:
   *
   * > Create a new field for your JSON schema
   */
  readonly fieldAddNewDescription: string;
  /**
   * The translation for the key `fieldAddNewBadge`. English default is:
   *
   * > Schema Builder
   */
  readonly fieldAddNewBadge: string;
  /**
   * The translation for the key `fieldAddNewCancel`. English default is:
   *
   * > Cancel
   */
  readonly fieldAddNewCancel: string;
  /**
   * The translation for the key `fieldAddNewConfirm`. English default is:
   *
   * > Add Field
   */
  readonly fieldAddNewConfirm: string;

  /**
   * The translation for the key `fieldTypeTextLabel`. English default is:
   *
   * > Text
   */
  readonly fieldTypeTextLabel: string;
  /**
   * The translation for the key `fieldTypeTextDescription`. English default is:
   *
   * > For text values like names, descriptions, etc.
   */
  readonly fieldTypeTextDescription: string;
  /**
   * The translation for the key `fieldTypeNumberLabel`. English default is:
   *
   * > Number
   */
  readonly fieldTypeNumberLabel: string;
  /**
   * The translation for the key `fieldTypeNumberDescription`. English default is:
   *
   * > For decimal or whole numbers
   */
  readonly fieldTypeNumberDescription: string;
  /**
   * The translation for the key `fieldTypeBooleanLabel`. English default is:
   *
   * > Yes/No
   */
  readonly fieldTypeBooleanLabel: string;
  /**
   * The translation for the key `fieldTypeBooleanDescription`. English default is:
   *
   * > For true/false values
   */
  readonly fieldTypeBooleanDescription: string;
  /**
   * The translation for the key `fieldTypeObjectLabel`. English default is:
   *
   * > Group
   */
  readonly fieldTypeObjectLabel: string;
  /**
   * The translation for the key `fieldTypeObjectDescription`. English default is:
   *
   * > For grouping related fields together
   */
  readonly fieldTypeObjectDescription: string;
  /**
   * The translation for the key `fieldTypeArrayLabel`. English default is:
   *
   * > List
   */
  readonly fieldTypeArrayLabel: string;
  /**
   * The translation for the key `fieldTypeArrayDescription`. English default is:
   *
   * > For collections of items
   */
  readonly fieldTypeArrayDescription: string;

  /**
   * The translation for the key `propertyDescriptionPlaceholder`. English default is:
   *
   * > Add description...
   */
  readonly propertyDescriptionPlaceholder: string;
  /**
   * The translation for the key `propertyDescriptionButton`. English default is:
   *
   * > Add description...
   */
  readonly propertyDescriptionButton: string;
  /**
   * The translation for the key `propertyRequired`. English default is:
   *
   * > Required
   */
  readonly propertyRequired: string;
  /**
   * The translation for the key `propertyOptional`. English default is:
   *
   * > Optional
   */
  readonly propertyOptional: string;
  /**
   * The translation for the key `propertyDelete`. English default is:
   *
   * > Delete field
   */
  readonly propertyDelete: string;

  /**
   * The translation for the key `arrayMinimumLabel`. English default is:
   *
   * > Minimum Items
   */
  readonly arrayMinimumLabel: string;
  /**
   * The translation for the key `arrayMinimumPlaceholder`. English default is:
   *
   * > No minimum
   */
  readonly arrayMinimumPlaceholder: string;
  /**
   * The translation for the key `arrayMaximumLabel`. English default is:
   *
   * > Maximum Items
   */
  readonly arrayMaximumLabel: string;
  /**
   * The translation for the key `arrayMaximumPlaceholder`. English default is:
   *
   * > No maximum
   */
  readonly arrayMaximumPlaceholder: string;
  /**
   * The translation for the key `arrayForceUniqueItemsLabel`. English default is:
   *
   * > Force unique items
   */
  readonly arrayForceUniqueItemsLabel: string;
  /**
   * The translation for the key `arrayItemTypeLabel`. English default is:
   *
   * > Item Type
   */
  readonly arrayItemTypeLabel: string;
  /**
   * The translation for the key `arrayValidationErrorMinMax`. English default is:
   *
   * > 'minItems' cannot be greater than 'maxItems'.
   */
  readonly arrayValidationErrorMinMax: string;
  /**
   * The translation for the key `arrayValidationErrorContainsMinMax`. English default is:
   *
   * > 'minContains' cannot be greater than 'maxContains'.
   */
  readonly arrayValidationErrorContainsMinMax: string;

  /**
   * The translation for the key `booleanAllowTrueLabel`. English default is:
   *
   * > Allow true value
   */
  readonly booleanAllowTrueLabel: string;
  /**
   * The translation for the key `booleanAllowFalseLabel`. English default is:
   *
   * > Allow false value
   */
  readonly booleanAllowFalseLabel: string;
  /**
   * The translation for the key `booleanNeitherWarning`. English default is:
   *
   * > Warning: You must allow at least one value.
   */
  readonly booleanNeitherWarning: string;

  /**
   * The translation for the key `numberMinimumLabel`. English default is:
   *
   * > Minimum Value
   */
  readonly numberMinimumLabel: string;
  /**
   * The translation for the key `numberMinimumPlaceholder`. English default is:
   *
   * > No minimum
   */
  readonly numberMinimumPlaceholder: string;
  /**
   * The translation for the key `numberMaximumLabel`. English default is:
   *
   * > Maximum Value
   */
  readonly numberMaximumLabel: string;
  /**
   * The translation for the key `numberMaximumPlaceholder`. English default is:
   *
   * > No maximum
   */
  readonly numberMaximumPlaceholder: string;
  /**
   * The translation for the key `numberExclusiveMinimumLabel`. English default is:
   *
   * > Exclusive Minimum
   */
  readonly numberExclusiveMinimumLabel: string;
  /**
   * The translation for the key `numberExclusiveMinimumPlaceholder`. English default is:
   *
   * > No exclusive min
   */
  readonly numberExclusiveMinimumPlaceholder: string;
  /**
   * The translation for the key `numberExclusiveMaximumLabel`. English default is:
   *
   * > Exclusive Maximum
   */
  readonly numberExclusiveMaximumLabel: string;
  /**
   * The translation for the key `numberExclusiveMaximumPlaceholder`. English default is:
   *
   * > No exclusive max
   */
  readonly numberExclusiveMaximumPlaceholder: string;
  /**
   * The translation for the key `numberMultipleOfLabel`. English default is:
   *
   * > Multiple Of
   */
  readonly numberMultipleOfLabel: string;
  /**
   * The translation for the key `numberMultipleOfPlaceholder`. English default is:
   *
   * > Any
   */
  readonly numberMultipleOfPlaceholder: string;
  /**
   * The translation for the key `numberAllowedValuesEnumLabel`. English default is:
   *
   * > Allowed Values (enum)
   */
  readonly numberAllowedValuesEnumLabel: string;
  /**
   * The translation for the key `numberAllowedValuesEnumNone`. English default is:
   *
   * > No restricted values set
   */
  readonly numberAllowedValuesEnumNone: string;
  /**
   * The translation for the key `numberAllowedValuesEnumAddPlaceholder`. English default is:
   *
   * > Add allowed value...
   */
  readonly numberAllowedValuesEnumAddPlaceholder: string;
  /**
   * The translation for the key `numberAllowedValuesEnumAddLabel`. English default is:
   *
   * > Add
   */
  readonly numberAllowedValuesEnumAddLabel: string;
  /**
   * The translation for the key `numberValidationErrorExclusiveMinMax`. English default is:
   *
   * > Minimum and maximum values must be consistent.
   */
  readonly numberValidationErrorMinMax: string;
  /**
   * The translation for the key `numberValidationErrorBothExclusiveAndInclusive`. English default is:
   *
   * > Both 'exclusiveMinimum' and 'minimum' cannot be set at the same time.
   */
  readonly numberValidationErrorBothExclusiveAndInclusiveMin: string;
  /**
   * The translation for the key `numberValidationErrorBothExclusiveAndInclusiveMax`. English default is:
   *
   * > Both 'exclusiveMaximum' and 'maximum' cannot be set at the same time.
   */
  readonly numberValidationErrorBothExclusiveAndInclusiveMax: string;
  /**
   * The translation for the key `numberValidationErrorEnumOutOfRange`. English default is:
   *
   * > Enum values must be within the defined range.
   */
  readonly numberValidationErrorEnumOutOfRange: string;

  /**
   * The translation for the key `objectPropertiesNone`. English default is:
   *
   * > No properties defined
   */
  readonly objectPropertiesNone: string;
  /**
   * The translation for the key `objectValidationErrorMinMax`. English default is:
   *
   * > 'minProperties' cannot be greater than 'maxProperties'.
   */
  readonly objectValidationErrorMinMax: string;

  /**
   * The translation for the key `stringMinimumLengthLabel`. English default is:
   *
   * > Minimum Length
   */
  readonly stringMinimumLengthLabel: string;
  /**
   * The translation for the key `stringMinimumLengthPlaceholder`. English default is:
   *
   * > No minimum
   */
  readonly stringMinimumLengthPlaceholder: string;
  /**
   * The translation for the key `stringMaximumLengthLabel`. English default is:
   *
   * > Maximum Length
   */
  readonly stringMaximumLengthLabel: string;
  /**
   * The translation for the key `stringMaximumLengthPlaceholder`. English default is:
   *
   * > No maximum
   */
  readonly stringMaximumLengthPlaceholder: string;
  /**
   * The translation for the key `stringPatternLabel`. English default is:
   *
   * > Pattern (regex)
   */
  readonly stringPatternLabel: string;
  /**
   * The translation for the key `stringPatternPlaceholder`. English default is:
   *
   * > ^[a-zA-Z]+$
   */
  readonly stringPatternPlaceholder: string;
  /**
   * The translation for the key `stringFormatLabel`. English default is:
   *
   * > Format
   */
  readonly stringFormatLabel: string;
  /**
   * The translation for the key `stringFormatNone`. English default is:
   *
   * > None
   */
  readonly stringFormatNone: string;
  /**
   * The translation for the key `stringFormatDateTime`. English default is:
   *
   * > Date-Time
   */
  readonly stringFormatDateTime: string;
  /**
   * The translation for the key `stringFormatDate`. English default is:
   *
   * > Date
   */
  readonly stringFormatDate: string;
  /**
   * The translation for the key `stringFormatTime`. English default is:
   *
   * > Time
   */
  readonly stringFormatTime: string;
  /**
   * The translation for the key `stringFormatEmail`. English default is:
   *
   * > Email
   */
  readonly stringFormatEmail: string;
  /**
   * The translation for the key `stringFormatUri`. English default is:
   *
   * > URI
   */
  readonly stringFormatUri: string;
  /**
   * The translation for the key `stringFormatUuid`. English default is:
   *
   * > UUID
   */
  readonly stringFormatUuid: string;
  /**
   * The translation for the key `stringFormatHostname`. English default is:
   *
   * > Hostname
   */
  readonly stringFormatHostname: string;
  /**
   * The translation for the key `stringFormatIpv4`. English default is:
   *
   * > IPv4 Address
   */
  readonly stringFormatIpv4: string;
  /**
   * The translation for the key `stringFormatIpv6`. English default is:
   *
   * > IPv6 Address
   */
  readonly stringFormatIpv6: string;
  /**
   * The translation for the key `stringAllowedValuesEnumLabel`. English default is:
   *
   * > Allowed Values (enum)
   */
  readonly stringAllowedValuesEnumLabel: string;
  /**
   * The translation for the key `stringAllowedValuesEnumNone`. English default is:
   *
   * > No restricted values set
   */
  readonly stringAllowedValuesEnumNone: string;
  /**
   * The translation for the key `stringAllowedValuesEnumAddPlaceholder`. English default is:
   *
   * > Add allowed value...
   */
  readonly stringAllowedValuesEnumAddPlaceholder: string;
  /**
   * The translation for the key `stringValidationErrorMinLength`. English default is:
   *
   * > 'minLength' cannot be greater than 'maxLength'.
   */
  readonly stringValidationErrorLengthRange: string;

  /**
   * The translation for the key `schemaTypeString`. English default is:
   *
   * > Text
   */
  readonly schemaTypeString: string;
  /**
   * The translation for the key `schemaTypeNumber`. English default is:
   *
   * > Number
   */
  readonly schemaTypeNumber: string;
  /**
   * The translation for the key `schemaTypeBoolean`. English default is:
   *
   * > Yes/No
   */
  readonly schemaTypeBoolean: string;
  /**
   * The translation for the key `schemaTypeObject`. English default is:
   *
   * > Object
   */
  readonly schemaTypeObject: string;
  /**
   * The translation for the key `schemaTypeArray`. English default is:
   *
   * > List
   */
  readonly schemaTypeArray: string;
  /**
   * The translation for the key `schemaTypeNull`. English default is:
   *
   * > Empty
   */
  readonly schemaTypeNull: string;

  /**
   * The translation for the key `schemaEditorTitle`. English default is:
   *
   * > JSON Schema Editor
   */
  readonly schemaEditorTitle: string;
  /**
   * The translation for the key `schemaEditorToggleFullscreen`. English default is:
   *
   * > Toggle fullscreen
   */
  readonly schemaEditorToggleFullscreen: string;
  /**
   * The translation for the key `schemaEditorEditModeVisual`. English default is:
   *
   * > Visual
   */
  readonly schemaEditorEditModeVisual: string;
  /**
   * The translation for the key `schemaEditorEditModeJson`. English default is:
   *
   * > JSON
   */
  readonly schemaEditorEditModeJson: string;

  /**
   * The translation for the key `inferrerTitle`. English default is:
   *
   * > Infer JSON Schema
   */
  readonly inferrerTitle: string;
  /**
   * The translation for the key `inferrerDescription`. English default is:
   *
   * > Paste your JSON document below to generate a schema from it.
   */
  readonly inferrerDescription: string;
  /**
   * The translation for the key `inferrerGenerate`. English default is:
   *
   * > Generate Schema
   */
  readonly inferrerGenerate: string;
  /**
   * The translation for the key `inferrerCancel`. English default is:
   *
   * > Cancel
   */
  readonly inferrerCancel: string;
  /**
   * The translation for the key `inferrerErrorInvalidJson`. English default is:
   *
   * > Invalid JSON format. Please check your input.
   */
  readonly inferrerErrorInvalidJson: string;

  /**
   * The translation for the key `validatorTitle`. English default is:
   *
   * > Validate JSON
   */
  readonly validatorTitle: string;
  /**
   * The translation for the key `validatorDescription`. English default is:
   *
   * > Paste your JSON document to validate against the current schema. Validation occurs automatically as you type.
   */
  readonly validatorDescription: string;
  /**
   * The translation for the key `validatorCurrentSchema`. English default is:
   *
   * > Current Schema:
   */
  readonly validatorCurrentSchema: string;
  /**
   * The translation for the key `validatorContent`. English default is:
   *
   * > Your JSON:
   */
  readonly validatorContent: string;
  /**
   * The translation for the key `validatorValid`. English default is:
   *
   * > JSON is valid according to the schema!
   */
  readonly validatorValid: string;
  /**
   * The translation for the key `validatorErrorInvalidSyntax`. English default is:
   *
   * > Invalid JSON syntax
   */
  readonly validatorErrorInvalidSyntax: string;
  /**
   * The translation for the key `validatorErrorSchemaValidation`. English default is:
   *
   * > Schema validation error
   */
  readonly validatorErrorSchemaValidation: string;
  /**
   * The translation for the key `validatorErrorCount`. English default is:
   *
   * > {count} validation errors detected
   */
  readonly validatorErrorCount: string;
  /**
   * The translation for the key `validatorErrorPathRoot`. English default is:
   *
   * > Root
   */
  readonly validatorErrorPathRoot: string;
  /**
   * The translation for the key `validatorErrorLocationLineAndColumn`. English default is:
   *
   * > Line {line}, Col {column}
   */
  readonly validatorErrorLocationLineAndColumn: string;
  /**
   * The translation for the key `validatorErrorLocationLineOnly`. English default is:
   *
   * > Line {line}
   */
  readonly validatorErrorLocationLineOnly: string;

  /**
   * The translation for the key `visualizerDownloadTitle`. English default is:
   *
   * > Download Schema
   */
  readonly visualizerDownloadTitle: string;
  /**
   * The translation for the key `visualizerDownloadFileName`. English default is:
   *
   * > schema.json
   */
  readonly visualizerDownloadFileName: string;
  /**
   * The translation for the key `visualizerSource`. English default is:
   *
   * > JSON Schema Source
   */
  readonly visualizerSource: string;

  /**
   * The translation for the key `visualEditorNoFieldsHint1`. English default is:
   *
   * > No fields defined yet
   */
  readonly visualEditorNoFieldsHint1: string;
  /**
   * The translation for the key `visualEditorNoFieldsHint2`. English default is:
   *
   * > Add your first field to get started
   */
  readonly visualEditorNoFieldsHint2: string;

  /**
   * The translation for the key `typeValidationErrorNegativeLength`. English default is:
   *
   * > Length values cannot be negative.
   */
  readonly typeValidationErrorNegativeLength: string;
  /**
   * The translation for the key `typeValidationErrorIntValue`. English default is:
   *
   * > Value must be an integer.
   */
  readonly typeValidationErrorIntValue: string;
  /**
   * The translation for the key `typeValidationErrorPositive`. English default is:
   *
   * > Value must be positive.
   */
  readonly typeValidationErrorPositive: string;
}
