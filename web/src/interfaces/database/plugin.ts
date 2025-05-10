export type ILLMTools = ILLMToolMetadata[];

export interface ILLMToolMetadata {
    name: string;
    description: string;
    parameters: ILLMToolParameters;
}

export interface ILLMToolParameters {
    properties: Map<string, ILLMToolParameter>;
    required: string[];
}

export interface ILLMToolParameter {
    type: string;
    description: string;
}